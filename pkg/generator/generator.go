// Copyright (C) 2022 Alibaba Cloud. All rights reserved.
//
// SPDX-License-Identifier: Apache-2.0

// The Nydus localdisk generator converts a Nydus image from the source registry
// to a gpt disk image file in localdisk format.

package generator

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
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

	for k, size := range image.layerSize {
		if k >= layerNum {
			break
		}

		partitionSectors = int64(math.Ceil(float64(size) / float64(blkSize)))
		partitionEnd = partitionSectors + partitionStart
		partitionName, partitionGUID := splitBlobid(image.layerDigest[k].Encoded()) // The 64-byte layer digest (blob id) is stored in two parts

		var part = gpt.Partition{Start: uint64(partitionStart), End: uint64(partitionEnd), Type: gpt.LinuxFilesystem, Name: partitionName, GUID: partitionGUID}
		partitions = append(partitions, &part)

		partitionStart = (partitionEnd + 2048) / 2048 * 2048
	}

	var imageDigest = image.imageManifest.GetDescriptor().Digest
	table = gpt.Table{
		Partitions:    partitions,
		GUID:          truncateBlobid(imageDigest.Encoded()),
		ProtectiveMBR: true,
	}
	log.Infof("Build GPT table with %d layers", len(partitions))

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

// Validate that the target image is consistent with the source image
func validateData(sourceImage string, targetImage string, layerNum int, downloadedBlobs []string) {
	var startTime = time.Now()
	log.Infof("Validating image %s in %s", sourceImage, targetImage)

	var disk, err = diskfs.Open(targetImage)
	if err != nil {
		log.Fatalln(err)
	}

	t, err := disk.GetPartitionTable()
	if err != nil {
		log.Fatalln(err)
	}

	// Validate each partition in the target image
	var errorCount = 0
	var parts = t.GetPartitions()
	for index, part := range parts {
		if index >= layerNum {
			break
		}

		// Compute the sha256 in the target image
		var targetHash = sha256.New()
		readLen, err := part.ReadContents(disk.File, targetHash)
		if readLen != int64(part.GetSize()) {
			log.Errorf("returned %d bytes written instead of %d", readLen, part.GetSize())
		}
		if err != nil {
			log.Errorf("returned error instead of nil")
			log.Fatalln(err)
		}
		var targetHashString = hex.EncodeToString(targetHash.Sum(nil))

		// Compute the sha256 in the source image
		blob, err := os.Open(downloadedBlobs[index])
		if err != nil {
			log.Fatal(err)
		}
		defer blob.Close()
		var sourceHash = sha256.New()
		if _, err := io.Copy(sourceHash, blob); err != nil {
			log.Fatal(err)
		}
		var sourceHashString = hex.EncodeToString(sourceHash.Sum(nil))

		if targetHashString == sourceHashString {
			log.Infof("The data in partition %d is correct, sha256: %s", index, targetHashString)
		} else {
			errorCount++
			log.Errorf("Data corrupted in partition %d, sha256: %s not equal %s", index, targetHashString, sourceHashString)
		}
	}

	if errorCount > 0 {
		log.Fatalf("The generated target image is corrupted with %d part, please try convert again", errorCount)
	}

	var elapsedTime = float32(time.Since(startTime)/time.Millisecond) / 1000
	log.Infof("Image validated in %.3f s", elapsedTime)
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

	var downloadedBlobs = downloadImage(image, targetDir)
	var layerCount = len(image.layerSize)
	var disk = buildDiskImage(image, filepath.Join(targetDir, outputImageName), layerCount)
	writeData(image, disk, layerCount)
	validateData(imagePath, disk.File.Name(), layerCount, downloadedBlobs)
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
func ConvertImage(imagePath string, workDir string, withTemps bool) {
	var startTime = time.Now()
	log.Infof("Starting convert localdisk image %s", imagePath)

	if withTemps {
		convertImageWithTemps(imagePath, workDir)
	} else {
		convertImage(imagePath, workDir)
	}

	var elapsedTime = float32(time.Since(startTime)/time.Millisecond) / 1000
	log.Infof("Converted image %s successfully in %.3f s", imagePath, elapsedTime)
}
