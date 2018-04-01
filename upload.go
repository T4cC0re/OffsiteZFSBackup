package main

import (
	"./Abstractions"
	"fmt"
	"os"
)

func uploadCommand() {
	uploader := Abstractions.NewUploader(os.Stdin, "btrfs", "/", *folder, *upload, *passphrase, *encryption, *authentication, *chunksize)
	meta, err := uploader.Upload()
	fmt.Fprintln(os.Stderr, meta, err)
}
