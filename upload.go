package main

import (
	"./GoogleDrive"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"github.com/pierrec/lz4"
	"github.com/satori/go.uuid"
	"io"
	"os"
	"strings"
	"time"
)

func panicIfNoPassphrase(decrypt bool) {
	if *passphrase == "" {
		if decrypt {
			panic(errors.New("must specify --passphrase for encrypted and/or authenticated backups"))
		} else {
			panic(errors.New("must specify --passphrase for encryption and/or authentication"))
		}
	}
}

func uploadCommand() {
	parent := GoogleDrive.FindOrCreateFolder(*folder)

	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		id, _ := uuid.NewV4()

		var writeTarget io.Writer

		iv := make([]byte, aes.BlockSize)
		n, err := rand.Read(iv)
		if n != aes.BlockSize || err != nil {
			panic(errors.New("failed to generate IV"))
		}

		encryptionL := strings.ToLower(*encryption)
		authenticationL := strings.ToLower(*authentication)

		inputMeta := &GoogleDrive.MetadataBase{Uuid: id.String(), FileName: *upload, IsData: true, Authentication: authenticationL, Encryption: encryptionL}

		uploader, err := GoogleDrive.NewGoogleDriveWriter(inputMeta, parent, *chunksize*1024*1024)
		if err != nil {
			panic(err)
		}
		defer uploader.Close()

		mac, keyStream := prepareMACAndEncryption(*passphrase, iv, inputMeta.Authentication, inputMeta.Encryption, false)
		if keyStream == nil {
			writeTarget = uploader
		} else {
			writeTarget = cipher.StreamWriter{S: *keyStream, W: uploader, Err: nil}
		}

		compress := lz4.NewWriter(writeTarget)
		compress.Header = lz4.Header{
			BlockDependency: true,
			BlockChecksum:   false,
			NoChecksum:      false,
			BlockMaxSize:    4 << 20,
			HighCompression: false,
		}
		defer compress.Close()

		writers = append(writers, compress)

		readProxy := &ReadProxy{os.Stdin, 0}

		fmt.Fprintf(os.Stderr, "Uploading as '%s'", inputMeta.Uuid)

		// Here the actual reading and upload begins
		_, err = io.Copy(io.MultiWriter(writers...), readProxy)
		if err != nil {
			panic(err)
		}

		err = compress.Close()
		if err != nil {
			panic(err)
		}

		err = uploader.Close()
		if err != nil {
			panic(err)
		}

		var authHMAC string
		if mac != nil {
			authHMAC = fmt.Sprintf("%x", (*mac).Sum(nil))
		}

		meta := &GoogleDrive.Metadata{
			HMAC:           authHMAC,
			IV:             fmt.Sprintf("%x", iv),
			FileName:       inputMeta.FileName,
			Uuid:           inputMeta.Uuid,
			Authentication: inputMeta.Authentication,
			Encryption:     inputMeta.Encryption,
			TotalSizeIn:    readProxy.Total,
			TotalSize:      uploader.Total,
			Chunks:         uploader.Chunk,
		}

		//Print summary:
		fmt.Fprintf(
			os.Stderr,
			"\nSummary:\n" +
				" - Filename: '%s'\n" +
				" - UUID: '%s'\n" +
				" - Crypto: %s with %s\n" +
				" - Bytes read: %d\n" +
				" - Bytes uploaded: %d (lz4 compressed)\n" +
				" - Chunks: %d\n",
				meta.FileName,
				meta.Uuid,
				meta.Encryption,
				meta.Authentication,
				meta.TotalSizeIn,
				meta.TotalSize,
				meta.Chunks,
			)

		for {
			fmt.Fprint(os.Stderr, "\033[2KUploading metadata...\r")
			if GoogleDrive.UploadMetadata(meta, parent) != nil {
				fmt.Fprint(os.Stderr, "\033[2KMetadata uploaded\n")
				break
			}
			fmt.Fprint(os.Stderr, "\033[2KFailed to upload metadata. Retrying...\r")
			time.Sleep(5 * time.Second)
		}
	}
}
