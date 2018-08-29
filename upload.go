package main

import (
	"./Abstractions"
	"github.com/prometheus/common/log"
	"os"
)

func uploadCommand() {
	uploader := Abstractions.NewUploader(os.Stdin, "btrfs", "/", *folder, *upload, *passphrase, *encryption, *authentication, *chunksize, *tmpdir)
	meta, err := uploader.Upload()
	log.Infoln(meta, err)
}
