package main

import (
	"./Abstractions"
	"./Common"
	"fmt"
	"os"
)

func downloadCommand() {
	uploader, err := Abstractions.NewDownloader(os.Stdout, *folder, *download, *passphrase)
	Common.PrintAndExitOnError(err ,1)
	meta, err := uploader.Download()
	fmt.Fprintln(os.Stderr, meta, err)
}
