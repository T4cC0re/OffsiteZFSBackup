package main

import (
	"./GoogleDrive"
	"fmt"
	"github.com/pierrec/lz4"
	"io"
	"os"
)

func downloadCommand() {
	parent := GoogleDrive.FindOrCreateFolder(*folder)
	reader, err := GoogleDrive.NewGoogleDriveReader(*filename, parent)
	if err != nil {
		panic(err)
	}

	defer reader.Close()

	zr := lz4.NewReader(nil)
	zr.Reset(reader)
	if _, err := io.Copy(os.Stdout, zr); err != nil {
		fmt.Fprintf(os.Stderr, "Error while decompressing input: %v", err)
	}
}
