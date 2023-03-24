# Nydus-localdisk
Nydus localdisk is a new backend implementation of [Nydus](https://github.com/dragonflyoss/image-service). 
This repo mainly contains tools and libraries related to localdisk image generation.

## Overview
The localdisk backend can store nydus images to local raw disk. In this backend, each Nydus mirror corresponds to a block device, each layer of the mirror is mapped to a partition of the block device, and the partition is located through the GPT partition table.

## Building

```
cd cmd/nydus-localdisk
go build
```

## Usage
Nydus localdisk generator converts a Nydus image from the source registry to a gpt disk image file in localdisk format. 

```
./nydus-localdisk convert --source $SOURCE --target $TARGET
```

If you only have an OCI image, you can use [Nydusify](https://github.com/dragonflyoss/image-service/blob/master/docs/nydusify.md) to convert it to a Nydus image first, and then use this tool to generate a Nydus localdisk image file.

## Import Library

```
import "github.com/adamqqqplay/nydus-localdisk/pkg/generator"

generator.ConvertImage(imagePath, workDir)
```

## Example
We provide a shell script to test localdisk image mounts, you can find it in `misc/test-localdisk.sh`

1. Build Nydus localdisk generator.
```
git clone https://github.com/adamqqqplay/nydus-localdisk ~/nydus-localdisk
cd ~/nydus-localdisk/cmd/nydus-localdisk
go build
mv ./nydus-localdisk ~/nydus-localdisk/
```

2. Build Nydus that supports the localdisk backend.
```
git clone https://github.com/adamqqqplay/image-service
cd image-service
git checkout nydusd-localdisk-backend
make release
cp ./target/release/nydus* ~/nydus-localdisk/
```

3. Generate and test localdisk image.
```
cd ~/nydus-localdisk/
sudo nerdctl run -d --restart=always -p 5000:5000 registry
sudo nydusify convert --fs-version 6 --source ubuntu --target localhost:5000/ubuntu:nydus-v6
sudo ./nydus-localdisk convert --source localhost:5000/ubuntu:nydus-v6 --target ./ubuntu
./misc/test-localdisk.sh ./ubuntu/output.img
```

If you see output like this, it means the test was successful.
```
INFO Localdisk initialized at /dev/loop8, has 6 patitions, GUID: 28ecb493-f52b-4a8b-aa40-39991f31c21d
```
