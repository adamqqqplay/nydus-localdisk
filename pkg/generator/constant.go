/*
 * Copyright (c) 2022. Nydus Developers. All rights reserved.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package generator

const (
	// import from Nydus Snapshotter
	ManifestOSFeatureNydus   = "nydus.remoteimage.v1"
	MediaTypeNydusBlob       = "application/vnd.oci.image.layer.nydus.blob.v1"
	BootstrapFileNameInLayer = "image/image.boot"

	blobPrefix      = "blob-"
	bootstrapPrefix = "bootstrap-"
	outputImageName = "output.img"
)
