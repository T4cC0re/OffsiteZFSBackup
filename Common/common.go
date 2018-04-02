package Common

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"errors"
	"fmt"
	"golang.org/x/crypto/sha3"
	"hash"
	"io"
	"os"
	"strings"
)

type SnapshotManager interface {
	CreateSnapshot(subvolume string) string
	IsAvailableLocally(snapshot string) bool
	ListLocalSnapshots() []string
	Stream(snapshot string, parentSnapshot string) (io.ReadCloser, error)
	Restore(targetSubvolume string) (io.WriteCloser, error)
}

func CreateHMAC(hash func() hash.Hash, passphrase string) hash.Hash {
	mac := hmac.New(hash, []byte(passphrase))
	return mac
}

func panicIfNoPassphrase(decrypt bool, passphrase string) {
	if passphrase == "" {
		if decrypt {
			panic(errors.New("must specify --passphrase for encrypted and/or authenticated backups"))
		} else {
			panic(errors.New("must specify --passphrase for encryption and/or authentication"))
		}
	}
}

func PrintAndExitOnError(err error, code int) {
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, err)
	os.Exit(code)
}

func PrepareMACAndEncryption(passphrase string, iv []byte, authentication string, encryption string, decrypt bool) (hash.Hash, cipher.Stream) {
	passwordHash := sha3.Sum256([]byte(passphrase))

	fmt.Fprintf(
		os.Stderr,
		"DEBUG:\n\t- passhash:\t%x\n\t- IV:\t\t%x\n\t- auth:\t\t%s\n\t- encryption:\t%s\n\t- decrypt:\t%v\n",
		passwordHash,
		iv,
		authentication,
		encryption,
		decrypt,
	)

	var block cipher.Block
	var err error
	if encryption != "none" {
		// Since passwordHash is a 256-bit/32-byte slice, this will set AES-256
		block, err = aes.NewCipher(passwordHash[:])
		if err != nil {
			panic(err)
		}
	}

	var mac hash.Hash
	switch authentication {
	case "none":
		mac = nil
	case "hmac-sha512":
		panicIfNoPassphrase(decrypt, passphrase)
		mac = CreateHMAC(sha512.New, passphrase)
	case "hmac-sha256":
		panicIfNoPassphrase(decrypt, passphrase)
		mac = CreateHMAC(sha256.New, passphrase)
	case "hmac-sha3-512":
		panicIfNoPassphrase(decrypt, passphrase)
		mac = CreateHMAC(sha3.New512, passphrase)
	case "hmac-sha3-256":
		panicIfNoPassphrase(decrypt, passphrase)
		mac = CreateHMAC(sha3.New256, passphrase)
	default:
		panic(errors.New("unsupported authentication method"))
	}

	var keyStream cipher.Stream
	switch encryption {
	case "none":
		keyStream = nil
	case "aes-ofb":
		panicIfNoPassphrase(decrypt, passphrase)
		keyStream = cipher.NewOFB(block, iv)
	case "aes-cfb":
		panicIfNoPassphrase(decrypt, passphrase)
		if decrypt {
			keyStream = cipher.NewCFBDecrypter(block, iv)
		} else {
			keyStream = cipher.NewCFBEncrypter(block, iv)
		}
	case "aes-ctr":
		panicIfNoPassphrase(decrypt, passphrase)
		keyStream = cipher.NewCTR(block, iv)
	default:
		panic(errors.New("unsupported encryption method"))
	}

	if keyStream != nil {
		fmt.Fprintf(os.Stderr, "Encryption enabled .....: %s\n", strings.ToUpper(encryption))
	}
	if mac != nil {
		fmt.Fprintf(os.Stderr, "Authentication enabled .: %s\n", strings.ToUpper(authentication))
	}

	return mac, keyStream
}
