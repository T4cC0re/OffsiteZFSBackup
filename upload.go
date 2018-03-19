package main

import (
	"./GoogleDrive"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"errors"
	"fmt"
	"github.com/pierrec/lz4"
	"github.com/satori/go.uuid"
	"golang.org/x/crypto/sha3"
	"hash"
	"io"
	"os"
	"strings"
)

func panicIfNoPassphrase() {
	if *passphrase == "" {
		panic(errors.New("Must specify passphrase for encryption and/or authentication"))
	}
}

func createHMAC(hash func() hash.Hash) hash.Hash {
	mac := hmac.New(hash, []byte(*passphrase))
	writers = append(writers, mac)
	return mac
}

var writers []io.Writer

func uploadCommand() {
	parent := GoogleDrive.FindOrCreateFolder(*folder)

	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		settings := StorageSettings{Encryption: *encryption, Authentication: *authentication}
		id, _ := uuid.NewV4()

		fmt.Fprintf(os.Stderr, "Uploading file encrypted with %s and authenticated by %s. UUID: %s\n", settings.Encryption, settings.Authentication, id.String())

		filename := generateFileBaseName(*filename, id.String(), settings, true)
		uploader, err := GoogleDrive.NewGoogleDriveWriter(filename, id.String(), parent, *chunksize*1024*1024)
		if err != nil {
			panic(err)
		}
		defer uploader.Close()

		compress := lz4.NewWriter(uploader)
		compress.Header = lz4.Header{
			BlockDependency: true,
			BlockChecksum:   false,
			NoChecksum:      false,
			BlockMaxSize:    4 << 20,
			HighCompression: false,
		}
		defer compress.Close()

		writers = append(writers, compress)

		var mac hash.Hash
		switch strings.ToLower(*authentication) {
		case "none":
		case "hmac-sha512":
			panicIfNoPassphrase()
			mac = createHMAC(sha512.New)
		case "hmac-sha256":
			panicIfNoPassphrase()
			mac = createHMAC(sha256.New)
		case "hmac-sha3-512":
			panicIfNoPassphrase()
			mac = createHMAC(sha3.New512)
		case "hmac-sha3-256":
			panicIfNoPassphrase()
			mac = createHMAC(sha3.New256)
		default:
			panic(errors.New("unsupported authentication method"))
		}

		_, err = io.Copy(io.MultiWriter(writers...), os.Stdin)
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
			authHMAC = fmt.Sprintf("%x", mac.Sum(nil))
		}

		fmt.Fprintf(os.Stderr, "HMAC: %s", authHMAC)

		//TODO: Write Metadata here!
	}
}
