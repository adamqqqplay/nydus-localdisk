/*
 * Copyright (c) 2022. Nydus Developers. All rights reserved.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package generator

import (
	"bufio"
	"context"
	"io"
	"os"

	"github.com/opencontainers/go-digest"
	"github.com/regclient/regclient"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/ref"
	log "github.com/sirupsen/logrus"
)

func downloadBlobByRef(client regclient.RegClient, imageRef ref.Ref, hash digest.Digest, path string) {
	var ctx = context.Background()
	blob, err := client.BlobGet(ctx, imageRef, types.Descriptor{Digest: hash})
	if err != nil {
		log.Fatalln(err)
	}
	defer blob.Close()

	newBlob, err := os.Create(path)
	if err != nil {
		log.Fatalln(err)
	}
	defer newBlob.Close()

	bufSize := 1024 * 1024 // 1MB buffer
	in := bufio.NewReaderSize(blob, bufSize)
	out := bufio.NewWriterSize(newBlob, bufSize)
	written, err := io.Copy(out, in)
	if err != nil {
		log.Fatalln(err)
	}
	out.Flush()

	log.Infof("Downloaded blob to: %s (%d Bytes)", path, written)
}

// reserved for compatibility
func downloadBlob(imagePath string, hash digest.Digest, path string) {
	var ctx = context.Background()
	var reference, err = ref.New(imagePath)
	if err != nil {
		log.Fatalln(err)
	}
	var client = regclient.New()
	defer client.Close(ctx, reference)

	downloadBlobByRef(*client, reference, hash, path)
}
