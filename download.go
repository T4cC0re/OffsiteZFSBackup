package main

import (
	"./Abstractions"
	"fmt"
	"os"
)

func downloadCommand() {
	uploader := Abstractions.NewDownloader(os.Stdout, *folder, *download, *passphrase)
	meta, err := uploader.Download()
	fmt.Fprintln(os.Stderr, meta, err)
}
