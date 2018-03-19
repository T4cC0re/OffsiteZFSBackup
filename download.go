package main

import (
	"./GoogleDrive"
	"fmt"
	"github.com/pierrec/lz4"
	"io"
	"os"
	"strings"
	"golang.org/x/crypto/sha3"
	"crypto/aes"
	"crypto/cipher"
)

func downloadCommand() {
	parent := GoogleDrive.FindOrCreateFolder(*folder)
	reader, err := GoogleDrive.NewGoogleDriveReader(*filename, parent)
	if err != nil {
		panic(err)
	}
	defer reader.Close()

	var read io.Reader
	var stream cipher.Stream
	var block cipher.Block

	passHash := sha3.Sum256([]byte(*passphrase))
	iv := passHash[:aes.BlockSize] // TODO: THIS IS REEEALLY BAD!!! Generate this on random after metadata stuff

	if strings.ToLower(*encryption) != "none" {
		block, err = aes.NewCipher(passHash[:])
		if err != nil {
			panic(err)
		}
	}

	switch strings.ToLower(*encryption) {
	default:
		fallthrough
	case "none":
		read = reader
	case "aes-ofb":
		panicIfNoPassphrase()
		stream = cipher.NewOFB(block, iv)
	case "aes-cfb":
		panicIfNoPassphrase()
		stream = cipher.NewCFBDecrypter(block, iv)
	case "aes-ctr":
		panicIfNoPassphrase()
		stream = cipher.NewCTR(block, iv)
	}

	if stream != nil {
		read = cipher.StreamReader{S:stream, R: reader}
	}

	fmt.Fprintf(os.Stderr, "LZ4MAGIC: %x\n", uint32(0x184D2204))

	zr := lz4.NewReader(nil)
	zr.Reset(read)
	if _, err := io.Copy(os.Stdout, zr); err != nil {
		fmt.Fprintf(os.Stderr, "Error while decompressing input: %v", err)
	}
}
