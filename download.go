package main

import (
	"gitlab.com/T4cC0re/OffsiteZFSBackup/Abstractions"
	"gitlab.com/T4cC0re/OffsiteZFSBackup/Common"
	"github.com/prometheus/common/log"
	"os"
)

func downloadCommand() {
	uploader, err := Abstractions.NewDownloader(&backend, os.Stdout, *folder, *download, *passphrase, *tmpdir)
	Common.PrintAndExitOnError(err, 1)
	meta, err := uploader.Download()
	log.Infoln(meta, err)
}
