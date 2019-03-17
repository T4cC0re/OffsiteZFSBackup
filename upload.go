package main

import (
	"gitlab.com/T4cC0re/OffsiteZFSBackup/Abstractions"
	"github.com/prometheus/common/log"
	"os"
)

func uploadCommand() {
	uploader := Abstractions.NewUploader(&backend, os.Stdin, "btrfs", "/", *folder, *upload, *passphrase, *encryption, *authentication, *chunksize, *tmpdir, *ratio)
	meta, err := uploader.Upload()
	log.Infoln(meta, err)
}
