package main

import (
	"./Abstractions"
	"./Common"
	"github.com/prometheus/common/log"
	"os"
)

func downloadCommand() {
	uploader, err := Abstractions.NewDownloader(os.Stdout, *folder, *download, *passphrase, *tmpdir)
	Common.PrintAndExitOnError(err, 1)
	meta, err := uploader.Download()
	log.Infoln(meta, err)
}
