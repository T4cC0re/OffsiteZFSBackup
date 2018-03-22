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
	metadata, err := GoogleDrive.FetchMetadata(*download, parent)
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

	writers = append(writers, os.Stdout)
	multiWriter := io.MultiWriter(writers...)

	zr := lz4.NewReader(nil)
	zr.Reset(read)
	if _, err := io.Copy(multiWriter, zr); err != nil {
		fmt.Fprintf(os.Stderr, "Error while decompressing input: %v", err)
	}

	hmac := fmt.Sprintf("%x", (*mac).Sum(nil))

	if metadata.HMAC != hmac {
		fmt.Fprintf(os.Stderr, "Crap. HMAC does not match... :(\nWanted:\t%s\nGot:\t%s\n", metadata.HMAC, hmac)
		os.Exit(1)
	}

	//Print summary:
	fmt.Fprintf(
		os.Stderr,
		"\nSummary:\n"+
			" - Filename: '%s'\n"+
			" - UUID: '%s'\n"+
			" - Crypto: %s with %s\n"+
			" - Bytes downloaded: %d (lz4 compressed)\n"+
			" - Bytes written: %d\n"+
			" - Chunks: %d\n",
		metadata.FileName,
		metadata.Uuid,
		metadata.Encryption,
		metadata.Authentication,
		metadata.TotalSize,
		metadata.TotalSizeIn, // Taken from metadata, because this has to mach or the HMAC wouldn't
		metadata.Chunks,
	)
}
