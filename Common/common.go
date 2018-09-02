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
	"golang.org/x/crypto/hkdf"
)

var E_INVALID_SNAPSHOT = errors.New("given input is not a valid snapshot")

type SnapshotManager interface {
	Cleanup(subvolume string, latestSnapshot string) ()
	CreateSnapshot(subvolume string) (string, error)
	IsAvailableLocally(snapshot string) bool
	ListLocalSnapshots() []string
	DeleteSnapshot(snapshot string) (bool, error)
	Stream(snapshot string, parentSnapshot string) (io.ReadCloser, error)
	Restore(targetSubvolume string) (io.WriteCloser, error)
}

func checkKeyLength(key []byte) {
	if len(key) != 32 {
		log.Fatal("key does not have the correct length. (did you specify --passphrase?)")

	}
}

func PrintAndExitOnError(err error, code int) {
	if err == nil {
		return
	}
	log.Fatalln(err)
	os.Exit(code)
}

func DeriveKeys(master []byte, salt []byte) (authentication []byte, encryption []byte) {
	// Non secret context specific info.
	info := []byte("OZB HKDF")

	derivationFunction := hkdf.New(sha3.New512, master, salt, info)

	authentication = make([]byte, 32)
	encryption = make([]byte, 32)

	n, err := io.ReadFull(derivationFunction, authentication)
	if n != 32 || err != nil {
		log.Fatal(err)
	}

	n, err = io.ReadFull(derivationFunction, encryption)
	if n != 32 || err != nil {
		log.Fatal(err)
	}
	return
}

func PrepareMACAndEncryption(authenticationKey []byte, encryptionKey []byte, iv []byte, authentication string, encryption string, decrypt bool) (hash.Hash, cipher.Stream) {
	var mac hash.Hash
	switch authentication {
	case "none":
		mac = nil
	case "hmac-sha512":
		checkKeyLength(authenticationKey)
		mac = hmac.New(sha512.New, authenticationKey)
	case "hmac-sha256":
		checkKeyLength(authenticationKey)
		mac = hmac.New(sha256.New, authenticationKey)
	case "hmac-sha3-512":
		checkKeyLength(authenticationKey)
		mac = hmac.New(sha3.New512, authenticationKey)
	case "hmac-sha3-256":
		checkKeyLength(authenticationKey)
		mac = hmac.New(sha3.New256, authenticationKey)
	default:
		log.Fatal("unsupported authentication method")
	}

	var keyStream cipher.Stream
	var block cipher.Block
	var err error
	if encryption != "none" {
		checkKeyLength(encryptionKey)
		// Since passwordHash is a 256-bit/32-byte slice, this will set AES-256
		block, err = aes.NewCipher(encryptionKey)
		if err != nil {
			log.Fatal(err)
		}
	}

	switch encryption {
	case "none":
		keyStream = nil
	case "aes-ofb":
		keyStream = cipher.NewOFB(block, iv)
	case "aes-cfb":
		if decrypt {
			keyStream = cipher.NewCFBDecrypter(block, iv)
		} else {
			keyStream = cipher.NewCFBEncrypter(block, iv)
		}
	case "aes-ctr":
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
