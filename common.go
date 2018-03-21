package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"errors"
	"golang.org/x/crypto/sha3"
	"hash"
	"io"
	"fmt"
	"os"
)

func createHMAC(hash func() hash.Hash) hash.Hash {
	mac := hmac.New(hash, []byte(*passphrase))
	writers = append(writers, mac)
	return mac
}

var writers []io.Writer

func prepareMACAndEncryption(passphrase string, iv []byte, authentication string, encryption string, decrypt bool) (*hash.Hash, *cipher.Stream) {
	passwordHash := sha3.Sum256([]byte(passphrase))

	var block cipher.Block
	var err error
	if encryption != "none" {
		block, err = aes.NewCipher(passwordHash[:])
		if err != nil {
			panic(err)
		}
	}

	var mac hash.Hash
	switch authentication {
	case "none":
	case "hmac-sha512":
		panicIfNoPassphrase(decrypt)
		mac = createHMAC(sha512.New)
	case "hmac-sha256":
		panicIfNoPassphrase(decrypt)
		mac = createHMAC(sha256.New)
	case "hmac-sha3-512":
		panicIfNoPassphrase(decrypt)
		mac = createHMAC(sha3.New512)
	case "hmac-sha3-256":
		panicIfNoPassphrase(decrypt)
		mac = createHMAC(sha3.New256)
	default:
		panic(errors.New("unsupported authentication method"))
	}

	var keyStream cipher.Stream
	switch encryption {
	case "none":
		keyStream = nil
	case "aes-ofb":
		panicIfNoPassphrase(decrypt)
		keyStream = cipher.NewOFB(block, iv)
	case "aes-cfb":
		panicIfNoPassphrase(decrypt)
		if decrypt {
			keyStream = cipher.NewCFBDecrypter(block, iv)
		} else {
			keyStream = cipher.NewCFBEncrypter(block, iv)
		}
	case "aes-ctr":
		panicIfNoPassphrase(decrypt)
		keyStream = cipher.NewCTR(block, iv)
	default:
		panic(errors.New("unsupported encryption method"))
	}

	if keyStream != nil {
		fmt.Fprintf(os.Stderr, "Encryption enabled: %s\n", encryption)
	}
	if mac != nil {
		fmt.Fprintf(os.Stderr, "Authentication enabled: %s\n", authentication)
	}

	return &mac, &keyStream
}
