// Copyright (C) 2022 Alibaba Cloud. All rights reserved.
//
// SPDX-License-Identifier: Apache-2.0

// The Nydus localdisk generator converts a Nydus image from the source registry
// to a gpt disk image file in localdisk format.

package main

import (
	"fmt"
	"os"

	"github.com/adamqqqplay/nydus-localdisk/pkg/generator"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

func printWelcome() {
	var str = `
    Welcome to use nydus-localdisk!
    License: Apache-2.0
    `

	fmt.Println(str)
}

func main() {
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
	})

	printWelcome()

	app := &cli.App{
		Name:  "nydus-localdisk",
		Usage: "provide Nydus localdisk backend support",
		Commands: []*cli.Command{
			{
				Name:  "convert",
				Usage: "converts a Nydus image from the source registry to a gpt disk image file in localdisk format.",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "source",
						Usage:    "source image reference (example: localhost:5000/ubuntu-nydus)",
						Required: true,
						EnvVars:  []string{"SOURCE"},
					},
					&cli.StringFlag{
						Name:     "target",
						Value:    "./workdir",
						Usage:    "target localdisk dir",
						Required: false,
						EnvVars:  []string{"TARGET"},
					},
				},
				Action: func(cCtx *cli.Context) error {
					var imagePath = cCtx.String("source")
					var workDir = cCtx.String("target")
					generator.ConvertImage(imagePath, workDir, false)

					return nil
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
