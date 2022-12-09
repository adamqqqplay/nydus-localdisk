# Nydus-localdisk
Nydus localdisk is a new backend implementation of [Nydus](https://github.com/dragonflyoss/image-service). 
This repo mainly contains tools and libraries related to localdisk image generation

## Overview
The localdisk backend can store nydus images to local raw disk. In this backend, each Nydus mirror corresponds to a block device, each layer of the mirror is mapped to a partition of the block device, and the partition is located through the GPT partition table.

## Building
> cd cmd/nydus-localdisk
> go build

## Usage
Nydus localdisk generator converts a Nydus image from the source registry to a gpt disk image file in localdisk format. 
> nydus-localdisk $IMAGE_PATH $TARGET_DIR

If you only have an OCI image, you can use [Nydusify](https://github.com/dragonflyoss/image-service/blob/master/docs/nydusify.md) to convert it to a Nydus image first, and then use this tool to generate a Nydus localdisk image file.

## Import Library

> import "github.com/adamqqqplay/nydus-localdisk/pkg/generator"
> 
> generator.ConvertImage(imagePath, workDir)