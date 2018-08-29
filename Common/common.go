package Common

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
	"os"
	"strings"
	"github.com/prometheus/common/log"
)

var E_INVALID_SNAPSHOT = errors.New("given input is not a valid snapshot")

type SnapshotManager interface {
	CreateSnapshot(subvolume string) (string, error)
	IsAvailableLocally(snapshot string) bool
	ListLocalSnapshots() []string
	DeleteSnapshot(snapshot string) (bool, error)
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
			log.Fatal("must specify --passphrase for encrypted and/or authenticated backups")
		} else {
			log.Fatal("must specify --passphrase for encryption and/or authentication")
		}
	}
}

func PrintAndExitOnError(err error, code int) {
	if err == nil {
		return
	}
	log.Fatalln(err)
	os.Exit(code)
}

func PrepareMACAndEncryption(passphrase string, iv []byte, authentication string, encryption string, decrypt bool) (hash.Hash, cipher.Stream) {
	passwordHash := sha3.Sum256([]byte(passphrase))

	//fmt.Fprintf(
	//	os.Stderr,
	//	"DEBUG:\n\t- passhash:\t%x\n\t- IV:\t\t%x\n\t- auth:\t\t%s\n\t- encryption:\t%s\n\t- decrypt:\t%v\n",
	//	passwordHash,
	//	iv,
	//	authentication,
	//	encryption,
	//	decrypt,
	//)

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
		log.Fatal("unsupported authentication method")
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
		log.Fatal("unsupported encryption method")
	}

	if keyStream != nil {
		log.Infof("Encryption enabled .....: %s", strings.ToUpper(encryption))
	}
	if mac != nil {
		log.Infof("Authentication enabled .: %s", strings.ToUpper(authentication))
	}

	return mac, keyStream
}
