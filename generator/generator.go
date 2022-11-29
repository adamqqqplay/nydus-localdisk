// Copyright (C) 2022 Alibaba Cloud. All rights reserved.
//
// SPDX-License-Identifier: Apache-2.0

// The Nydus localdisk generator converts a Nydus image from the source registry
// to a gpt disk image file in localdisk format.

package generator

import (
	"bufio"
	"context"
	"io"
	"math"
	"os"
	"time"

	"github.com/diskfs/go-diskfs"
	"github.com/diskfs/go-diskfs/partition/gpt"
	"github.com/docker/distribution/reference"
	"github.com/opencontainers/go-digest"
	"github.com/regclient/regclient"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/manifest"
	"github.com/regclient/regclient/types/ref"
	log "github.com/sirupsen/logrus"
)

type imageInfo struct {
	imagePath   string
	imageParser manifest.Imager
	layerDigest []digest.Digest
	layerSize   []int64
	totalSize   int64
}

func getImageParser(imagePath string) manifest.Imager {
	var ctx = context.Background()
	var client = regclient.New()
	var reference, err = ref.New(imagePath)
	if err != nil {
		log.Fatalln(err)
	}

	mani, err := client.ManifestGet(ctx, reference)
	if err != nil {
		log.Fatalln(err)
	}
	img, ok := mani.(manifest.Imager)
	if !ok {
		log.Fatalln("manifest must be an image")
	}
	return img
}

func getImageInfo(imagePath string) imageInfo {
	var imageParser = getImageParser(imagePath)

	layers, err := imageParser.GetLayers()
	if err != nil {
		log.Fatalln(err)
	}

	var totalSize int64 = 0
	var layerDigest []digest.Digest
	var layerSize []int64
	for _, v := range layers {
		if v.MediaType == "application/vnd.oci.image.layer.v1.tar+gzip" {
			layerDigest = append([]digest.Digest{v.Digest}, layerDigest...)
			layerSize = append([]int64{v.Size}, layerSize...)
			totalSize += v.Size
		} else if v.MediaType == "application/vnd.oci.image.layer.nydus.blob.v1" {
			layerDigest = append(layerDigest, v.Digest)
			layerSize = append(layerSize, int64(v.Size))
			totalSize += int64(v.Size)
		}
	}

	var image = imageInfo{
		imagePath,
		imageParser,
		layerDigest,
		layerSize,
		totalSize,
	}

	return image
}

func downloadBlob(imagePath string, hash digest.Digest, size int64, path string) {
	ctx := context.Background()
	rc := regclient.New()
	reff, err := ref.New(imagePath)
	if err != nil {
		log.Fatalln(err)
	}

	reader, err := rc.BlobGet(ctx, reff, types.Descriptor{Digest: hash, Size: size})
	if err != nil {
		log.Fatalln(err)
	}
	contents, err := io.ReadAll(reader)
	if err != nil {
		log.Fatalln(err)
	}

	err = os.WriteFile(path, contents, 0666)
	if err != nil {
		log.Fatalln(err)
	}
	log.Infof("Downloaded blob to: %s", path)
}

func buildDiskTable(image imageInfo) gpt.Table {
	var partitions []*gpt.Partition
	var table gpt.Table

	var blkSize int64 = 512
	var partitionStart int64 = 2048
	var partitionSectors int64
	var partitionEnd int64
	var partitionName string

	for k, size := range image.layerSize {
		partitionSectors = size / blkSize
		partitionEnd = partitionSectors + partitionStart
		partitionName = string(image.layerDigest[k][7 : 32+7])

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

func getBlobFileName(d digest.Digest) string {
	var str = "blob-" + d.Encoded()
	return str
}

func getBootstrapFileName(d digest.Digest) string {
	var str = "bootstrap-" + d.Encoded()
	return str
}

// Round up x to a multiple of a, for example: x=6, a=4, the return value is 8
func alignUp(x, a int64) int64 {
	return (x + a - 1) &^ (a - 1)
}

func roundUp(value float64, nearest float64) float64 {
	return math.Ceil(value/nearest) * nearest
}

func writeData(image imageInfo, targetDir string) {

	var outputPath = targetDir + "/output.img"
	log.Infof("Prepare Write datas to localdisk image file")

	var inputSize = image.totalSize + int64(len(image.layerSize)+1)*1024*1024
	var diskSize int64 = int64(roundUp(float64(inputSize), 512))
	var diskSize2 int64 = alignUp(inputSize, 512)
	if diskSize != diskSize2 {
		log.Fatalf("internal error: diskSize %d not equal %d", diskSize, diskSize2)
	}

	log.Infof("Align input data size %d Byte to a multiple of disk sector size (512 Byte): %d Byte", inputSize, diskSize)

	// create a disk image
	var disk, err = diskfs.Create(outputPath, diskSize, diskfs.Raw)
	if err != nil {
		log.Fatalln(err)
	}

	// create a partition table
	var table = buildDiskTable(image)

	// apply the partition table
	err = disk.Partition(&table)
	if err != nil {
		log.Fatalln(err)
	}

	t, err := disk.GetPartitionTable()
	if err != nil {
		log.Fatalln(err)
	}
	var parts = t.GetPartitions()

	var prefix = targetDir
	for k, v := range image.layerDigest {
		var part = parts[k]
		var filename string

		if k == 0 {
			filename = "/" + getBootstrapFileName(v)
		} else {
			filename = "/" + getBlobFileName(v)
		}

		err = os.Truncate(prefix+filename, part.GetSize())
		if err != nil {
			log.Fatalln(err)
		}

		f, err := os.Open(prefix + filename)
		if err != nil {
			log.Fatalln(err)
		}
		reader := bufio.NewReader(f)

		file, err := os.OpenFile(outputPath, os.O_CREATE|os.O_RDWR, os.ModeAppend|os.ModePerm)
		if err != nil {
			log.Fatalln(err)
		}

		written, err := part.WriteContents(file, reader)
		if written != uint64(part.GetSize()) {
			log.Errorf("returned %d bytes written instead of %d", written, part.GetSize())
		}
		if err != nil {
			log.Errorf("returned error instead of nil")
			log.Fatalln(err)
		}
	}

	log.Infof("Localdisk image file has been written in: %s", outputPath)

}

func downloadImage(image imageInfo, targetDir string) {
	log.Infof("Download blobs in %s", targetDir)
	var err = os.RemoveAll(targetDir)
	if err != nil {
		log.Fatalln(err)
	}
	err = os.MkdirAll(targetDir, 0766)
	if err != nil {
		log.Fatalln(err)
	}

	for k, v := range image.layerDigest {
		if k == 0 {
			downloadBlob(image.imagePath, v, int64(image.layerSize[k]), targetDir+"/"+getBootstrapFileName(v))
		} else {
			downloadBlob(image.imagePath, v, int64(image.layerSize[k]), targetDir+"/"+getBlobFileName(v))
		}
	}
	log.Infof("Downloaded %d blobs successfully", len(image.layerDigest))
}

// TODO
func validateData(sourceImage string, validateImage string) {

}

func generateTargetDir(image imageInfo, targetDir string) string {
	var parsed, err = reference.ParseNormalizedNamed(image.imagePath)
	if err != nil {
		log.Fatalln(err)
	}

	var path = targetDir + "/" + reference.Path(parsed)

	return path
}

func convertImage(imagePath string, workDir string) {
	var image = getImageInfo(imagePath)
	var targetDir = generateTargetDir(image, workDir)

	downloadImage(image, targetDir)
	writeData(image, targetDir)
	validateData(imagePath, targetDir)
}

func ConvertImage(imagePath string, workDir string) {
	var startTime = time.Now()
	log.Infof("Starting convert localdisk image %s", imagePath)

	convertImage(imagePath, workDir)

	var elapsedTime = float32(time.Since(startTime)/time.Millisecond) / 1000
	log.Infof("Converted image %s successfully in %.3f s", imagePath, elapsedTime)
}
