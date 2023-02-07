/*
 * Copyright (c) 2022. Nydus Developers. All rights reserved.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package generator

import (
	"context"

	"github.com/opencontainers/go-digest"
	"github.com/regclient/regclient"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/manifest"
	"github.com/regclient/regclient/types/ref"
	log "github.com/sirupsen/logrus"
)

type imageInfo struct {
	imagePath     string
	imageManifest manifest.Manifest
	layerDigest   []digest.Digest
	layerSize     []int64
	totalSize     int64
}

func getImageManifest(imagePath string) manifest.Manifest {
	var ctx = context.Background()
	var reference, err = ref.New(imagePath)
	if err != nil {
		log.Fatalln(err)
	}
	var client = regclient.New()
	defer client.Close(ctx, reference)

	mani, err := client.ManifestGet(ctx, reference)
	if err != nil {
		log.Fatalln(err)
	}
	return mani
}

func getImageInfo(imagePath string) imageInfo {
	var imageManifest = getImageManifest(imagePath)

	imageParser, ok := imageManifest.(manifest.Imager)
	if !ok {
		log.Fatalln("manifest must be an image")
	}

	layers, err := imageParser.GetLayers()
	if err != nil {
		log.Fatalln(err)
	}

	var totalSize int64 = 0
	var layerDigest []digest.Digest
	var layerSize []int64
	for _, v := range layers {
		if v.MediaType == types.MediaTypeOCI1LayerGzip {
			// Let the bootstrap data be saved in the front position
			layerDigest = append([]digest.Digest{v.Digest}, layerDigest...)
			layerSize = append([]int64{v.Size}, layerSize...)
		} else if v.MediaType == MediaTypeNydusBlob {
			layerDigest = append(layerDigest, v.Digest)
			layerSize = append(layerSize, v.Size)
		}
		totalSize += v.Size
	}

	var image = imageInfo{
		imagePath,
		imageManifest,
		layerDigest,
		layerSize,
		totalSize,
	}

	return image
}
