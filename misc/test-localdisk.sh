#!/bin/bash
#
# Copyright (C) 2022 Nydus Developers. All rights reserved.
#
# SPDX-License-Identifier: Apache-2.0

TEMP_DEVICE=/dev/loop8

function echoInfo() {
    echo ""
    echo "Nydus localdisk image mount test v1.0"
    echo "SPDX-License-Identifier: Apache-2.0"
    echo ""
}

function checkArgs() {
    if [ $# != 1 ]; then
        echo "ERROR: Parameter 'localdisk image path' required, such as: $0 ./output.img"
        exit
    else
        image_path=$1
    fi
}

function checkNydusd() {
    if ! [ -f "./nydusd" ]; then
        echo 'ERROR: nydusd binary is not exists, please compile first'
        exit
    fi
}

function umountDevice() {
    #if grep -qs $TEMP_DEVICE /proc/mounts; then
    sudo kpartx -dv $TEMP_DEVICE
    sudo losetup -d $TEMP_DEVICE
    #fi
}

function mountImage() {
    sudo losetup $TEMP_DEVICE "$image_path"
    sudo losetup -a
    sudo partprobe $TEMP_DEVICE
}

function writeConfig() {
    
    sudo tee /etc/nydusd-localdisk.json >/dev/null <<EOF
{
	"device": {
		"backend": {
			"type": "localdisk",
			"config": {
				"device_path": "$TEMP_DEVICE"
			}
		},
		"cache": {
			"type": "blobcache",
			"config": {
				"work_dir": "cache"
			}
		}
	},
	"mode": "direct",
	"digest_validate": false,
	"iostats_files": false,
	"enable_xattr": true,
	"fs_prefetch": {
		"enable": true,
		"threads_count": 4
	}
}
EOF

}

function startNydusd() {
    sudo dd if="$TEMP_DEVICE"p1 of=./bootstrap.tar.gz
    tar -xzf ./bootstrap.tar.gz

    WK_DIR=$(pwd)
    mkdir ./mnt

    sudo ./nydusd \
        --config /etc/nydusd-localdisk.json \
        --mountpoint "$WK_DIR"/mnt \
        --bootstrap "$WK_DIR"/image/image.boot \
        --log-level debug
}

echoInfo
checkArgs "$@"
checkNydusd
umountDevice
mountImage
writeConfig
startNydusd
umountDevice
