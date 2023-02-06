// Copyright (C) 2022 Alibaba Cloud. All rights reserved.
//
// SPDX-License-Identifier: Apache-2.0

// The Nydus localdisk generator converts a Nydus image from the source registry
// to a gpt disk image file in localdisk format.

package generator

import (
	"bufio"
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/diskfs/go-diskfs"
	"github.com/diskfs/go-diskfs/disk"
	"github.com/diskfs/go-diskfs/partition/gpt"
	"github.com/docker/distribution/reference"
	"github.com/opencontainers/go-digest"
	"github.com/regclient/regclient"
	"github.com/regclient/regclient/types/ref"
	log "github.com/sirupsen/logrus"
)

// Round up x to a multiple of a, for example: x=6, a=4, the return value is 8
func alignUp(x, a int64) int64 {
	return (x + a - 1) &^ (a - 1)
}

func roundUp(value float64, nearest float64) float64 {
	return math.Ceil(value/nearest) * nearest
}

func getBlobFileName(d digest.Digest) string {
	var str = blobPrefix + d.Encoded()
	return str
}

func getBootstrapFileName(d digest.Digest) string {
	var str = bootstrapPrefix + d.Encoded()
	return str
}

// layerNum indicates the number of layers used to build the gpt partition table.
// Nydus image have at least 2 layers, so layerNum must be >=2
func buildDiskTable(image imageInfo, layerNum int) gpt.Table {
	var partitions []*gpt.Partition
	var table gpt.Table

	var blkSize int64 = 512
	var partitionStart int64 = 2048
	var partitionSectors int64
	var partitionEnd int64
	var partitionName string
	const digestStorageLength int32 = 32

	for k, size := range image.layerSize {
		if k >= layerNum {
			break
		}

		partitionSectors = int64(math.Ceil(float64(size) / float64(blkSize)))
		partitionEnd = partitionSectors + partitionStart
		partitionName = image.layerDigest[k].Encoded()[:digestStorageLength]

		var part = gpt.Partition{Start: uint64(partitionStart), End: uint64(partitionEnd), Type: gpt.MicrosoftBasicData, Name: partitionName}
		partitions = append(partitions, &part)

		partitionStart = (partitionEnd + 2048) / 2048 * 2048
	}

	log.Infof("Build GPT table with %d layers", len(partitions))
	table = gpt.Table{
		Partitions:    partitions,
		ProtectiveMBR: true,
	}

	return table
}

// Build disk image with GPT table
func buildDiskImage(image imageInfo, outputPath string, layerNum int) *disk.Disk {

	log.Infof("Prepare Write datas to localdisk image file")

	var inputSize = image.totalSize + int64(len(image.layerSize)+1)*1024*1024
	var diskSize int64 = int64(roundUp(float64(inputSize), 512))
	var diskSize2 int64 = alignUp(inputSize, 512)
	if diskSize != diskSize2 {
		log.Fatalf("internal error: diskSize %d not equal %d", diskSize, diskSize2)
	}

	log.Infof("Align input data size %d Byte to a multiple of disk sector size (512 Byte): %d Byte", inputSize, diskSize)

	// create a disk image
	var newDisk, err = diskfs.Create(outputPath, diskSize, diskfs.Raw)
	if err != nil {
		log.Fatalln(err)
	}

	// create a partition table
	var table = buildDiskTable(image, layerNum)

	// apply the partition table
	err = newDisk.Partition(&table)
	if err != nil {
		log.Fatalln(err)
	}

	return newDisk
}

// Write blobs to each partitions in disk image
func writeData(image imageInfo, disk *disk.Disk, layerNum int) {

	t, err := disk.GetPartitionTable()
	if err != nil {
		log.Fatalln(err)
	}
	var parts = t.GetPartitions()

	var dir, _ = filepath.Split(disk.File.Name())

	for k, v := range image.layerDigest {
		if k >= layerNum {
			break
		}

		var part = parts[k]
		var fileName string

		if k == 0 {
			fileName = filepath.Join(dir, getBootstrapFileName(v))
		} else {
			fileName = filepath.Join(dir, getBlobFileName(v))
		}

		err = os.Truncate(fileName, part.GetSize())
		if err != nil {
			log.Fatalln(err)
		}

		f, err := os.Open(fileName)
		if err != nil {
			log.Fatalln(err)
		}
		reader := bufio.NewReader(f)

		written, err := part.WriteContents(disk.File, reader)
		if written != uint64(part.GetSize()) {
			log.Errorf("returned %d bytes written instead of %d", written, part.GetSize())
		}
		if err != nil {
			log.Errorf("returned error instead of nil")
			log.Fatalln(err)
		}
	}

	log.Infof("Localdisk image file has been written in: %s", disk.File.Name())

}

// Remove and make dir at path
func prepareDir(path string) error {
	var err = os.RemoveAll(path)
	if err != nil {
		return err
	}
	err = os.MkdirAll(path, 0766)
	if err != nil {
		return err
	}

	return nil
}

// Download Nydus image blobs from image.imagePath into targetDir, return downloaded blob paths
func downloadImage(image imageInfo, targetDir string) (downloadedBlobs []string) {
	var startTime = time.Now()
	log.Infof("Download blobs in %s", targetDir)

	var err = prepareDir(targetDir)
	if err != nil {
		log.Fatalln(err)
	}

	var layerCount = len(image.layerDigest)

	var ctx = context.Background()
	imageRef, err := ref.New(image.imagePath)
	if err != nil {
		log.Fatalln(err)
	}
	var client = regclient.New()
	defer client.Close(ctx, imageRef)

	var wg sync.WaitGroup
	wg.Add(layerCount)
	for idx, hash := range image.layerDigest {
		var targetPath string
		if idx == 0 {
			targetPath = filepath.Join(targetDir, getBootstrapFileName(hash))
		} else {
			targetPath = filepath.Join(targetDir, getBlobFileName(hash))
		}

		go func(hash digest.Digest) {
			//downloadBlob(image.imagePath, hash, targetPath)
			downloadBlobByRef(*client, imageRef, hash, targetPath)
			wg.Done()
		}(hash)

		downloadedBlobs = append(downloadedBlobs, targetPath)
	}
	wg.Wait()
	var elapsedTime = float32(time.Since(startTime)/time.Millisecond) / 1000
	log.Infof("Downloaded %d blobs successfully in %.3f s", layerCount, elapsedTime)

	return downloadedBlobs
}

// Download Nydus image blobs from imagePath into targetDir, return downloaded blob paths
func DownloadImage(imagePath string, targetDir string) (downloadedBlobs []string) {
	var image = getImageInfo(imagePath)
	downloadedBlobs = downloadImage(image, targetDir)
	return downloadedBlobs
}

// TODO
func validateData(sourceImage string, validateImage string) {

}

// Unused function
func generateTargetDir(image imageInfo, targetDir string) string {
	var parsed, err = reference.ParseNormalizedNamed(image.imagePath)
	if err != nil {
		log.Fatalln(err)
	}

	var path = filepath.Join(targetDir, reference.Path(parsed))

	return path
}

func convertImage(imagePath string, workDir string) {
	var image = getImageInfo(imagePath)
	//var targetDir = generateTargetDir(image, workDir)
	var targetDir = workDir

	downloadImage(image, targetDir)
	var layerCount = len(image.layerSize)
	var disk = buildDiskImage(image, filepath.Join(targetDir, outputImageName), layerCount)
	writeData(image, disk, layerCount)
	validateData(imagePath, disk.File.Name())
}

func convertImageWithTemps(imagePath string, workDir string) {
	var image = getImageInfo(imagePath)
	var targetDir = workDir

	downloadImage(image, targetDir)
	var layerCount = len(image.layerSize)
	var disk *disk.Disk
	for i := 2; i < layerCount+1; i++ {
		disk = buildDiskImage(image, filepath.Join(targetDir, outputImageName+".part"+fmt.Sprint(i-1)), i)
		writeData(image, disk, i)
	}

	dir, file := filepath.Split(disk.File.Name())
	err := os.Symlink(file, dir+outputImageName)
	if err != nil {
		log.Fatalln(err)
	}
}

// Pack Nydus image from imagePath into localdisk format at workDir/output.img
func ConvertImage(imagePath string, workDir string) {
	var startTime = time.Now()
	log.Infof("Starting convert localdisk image %s", imagePath)

	convertImageWithTemps(imagePath, workDir)

	var elapsedTime = float32(time.Since(startTime)/time.Millisecond) / 1000
	log.Infof("Converted image %s successfully in %.3f s", imagePath, elapsedTime)
}
