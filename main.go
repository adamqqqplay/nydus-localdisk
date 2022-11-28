// Copyright (C) 2022 Alibaba Cloud. All rights reserved.
//
// SPDX-License-Identifier: Apache-2.0

// The Nydus localdisk generator converts a Nydus image from the source registry
// to a gpt disk image file in localdisk format.

package main

import (
	"errors"
	"fmt"
	"nydus-localdisk/generator"
	"os"

	log "github.com/sirupsen/logrus"
)

func printWelcome() {
	var str = `
    Nydus localdisk generator v0.0.1 (golang)
    Usage: nydus-localdisk $IMAGE_PATH $TARGET_DIR
    License: Apache-2.0
    `

	fmt.Println(str)
}

const defaultTargetDir string = "./workdir/"

func readCommandArgs() (imagePath string, targetDir string, err error) {
	var args = os.Args
	var argNum = len(args)

	if argNum < 2 {
		var str = fmt.Sprintf("Argument $IMAGE_PATH required, such as: %s localhost:5000/ubuntu-nydus", args[0])
		err = errors.New(str)
		return "", "", err
	} else if argNum >= 4 {
		var str = fmt.Sprintf("%d arguments are too many for this program, up to 2 arguments required", argNum-1)
		err = errors.New(str)
		return "", "", err
	} else if argNum == 3 {
		imagePath = args[1]
		targetDir = args[2]
	} else if argNum == 2 {
		log.Infof("$TARGET_DIR not provided, use default dir: %s", defaultTargetDir)
		imagePath = args[1]
		targetDir = defaultTargetDir
	}

	return imagePath, targetDir, nil
}

func main() {
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
	})

	printWelcome()
	var imagePath, workDir, err = readCommandArgs()
	if err != nil {
		log.Fatalln(err)
	}

	generator.ConvertImage(imagePath, workDir)
}
