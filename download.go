package main

import (
	"./GoogleDrive"
	"crypto/cipher"
	"encoding/hex"
	"fmt"
	"github.com/pierrec/lz4"
	"io"
	"os"
)

func downloadCommand() {
	parent := GoogleDrive.FindOrCreateFolder(*folder)

	fmt.Fprintln(os.Stderr, "Fetching metadata...")
	metadata, err := GoogleDrive.FetchMetadata(*filename, parent)
	if err != nil {
		panic(err)
	}

	var read io.Reader

	iv, _ := hex.DecodeString(metadata.IV)

	mac, keyStream := prepareMACAndEncryption(*passphrase, iv, metadata.Authentication, metadata.Encryption, true)

	downloader, err := GoogleDrive.NewGoogleDriveReader(metadata)
	if err != nil {
		panic(err)
	}
	defer downloader.Close()

	if keyStream != nil {
		read = cipher.StreamReader{S: *keyStream, R: downloader}
	}
	if mac != nil {
	}

	// TODO: HMAC Verification

	zr := lz4.NewReader(nil)
	zr.Reset(read)
	if _, err := io.Copy(os.Stdout, zr); err != nil {
		fmt.Fprintf(os.Stderr, "Error while decompressing input: %v", err)
	}
}
