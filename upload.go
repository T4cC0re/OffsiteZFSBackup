package main

import (
	"./GoogleDrive"
	"crypto/aes"
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
	"crypto/cipher"
)

func panicIfNoPassphrase() {
	if *passphrase == "" {
		panic(errors.New("must specify passphrase for encryption and/or authentication"))
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

		var writeTarget io.Writer
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
			writeTarget = uploader
		case "aes-ofb":
			panicIfNoPassphrase()
			stream = cipher.NewOFB(block, iv)
		case "aes-cfb":
			panicIfNoPassphrase()
			stream = cipher.NewCFBEncrypter(block, iv)
		case "aes-ctr":
			panicIfNoPassphrase()
			stream = cipher.NewCTR(block, iv)
		}

		if stream != nil {
			writeTarget = cipher.StreamWriter{S:stream, W: uploader, Err: nil}
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

		meta := GoogleDrive.Metadata{
			HMAC:           authHMAC,
			IV:             fmt.Sprintf("%x", iv),
			FileName:       filename,
			Uuid:           id.String(),
			Authentication: *authentication,
			Encryption:     *encryption,
		}

		ok := GoogleDrive.UploadMetadata(&meta)
		if ok == nil {
			panic(errors.New("metadata write failed"))
		}

		//TODO: Write Metadata here!
	}
}
