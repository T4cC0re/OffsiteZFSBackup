package Abstractions

import (
	"../Common"
	"../GoogleDrive"
	"crypto/cipher"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/pierrec/lz4"
	"hash"
	"io"
	"os"
	"strings"
)

type Downloader struct {
	metadata    *GoogleDrive.Metadata
	multiWriter io.Writer
	readProxy   *ReadProxy
	zr          *lz4.Reader
	mac         hash.Hash
	keyStream   cipher.Stream
	downloader  *GoogleDrive.Reader
	timestamp   int64
	fileType    string
}

var E_HMAC_MISMATCH = errors.New("HMACs do not match. File has been tampered with, or was not transferred correctly")

func NewDownloader(w io.Writer, folder string, filename string, passphrase string) *Downloader {
	this := &Downloader{}

	var writers []io.Writer

	parent := GoogleDrive.FindOrCreateFolder(folder)

	fmt.Fprintln(os.Stderr, "Fetching metadata...")
	var err error
	this.metadata, err = GoogleDrive.FetchMetadata(filename, parent)
	if err != nil {
		panic(err)
	}

	var read io.Reader

	iv, _ := hex.DecodeString(this.metadata.IV)
	this.mac, this.keyStream = Common.PrepareMACAndEncryption(passphrase, iv, this.metadata.Authentication, this.metadata.Encryption, true)

	this.downloader, err = GoogleDrive.NewGoogleDriveReader(this.metadata)
	if err != nil {
		panic(err)
	}

	if this.keyStream != nil {
		read = cipher.StreamReader{S: this.keyStream, R: this.downloader}
	} else {
		read = this.downloader
	}
	if this.mac != nil {
		writers = append(writers, this.mac)
	}

	writers = append(writers, w)
	this.multiWriter = io.MultiWriter(writers...)

	this.zr = lz4.NewReader(nil)
	this.zr.Reset(read)

	return this
}

func (this *Downloader) close() error {
	return this.downloader.Close()
}

func (this *Downloader) Download() (*GoogleDrive.Metadata, error) {
	if _, err := io.Copy(this.multiWriter, this.zr); err != nil {
		return nil, err
	}

	err := this.close()

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}

	var hmac string
	if this.mac != nil {
		hmac = fmt.Sprintf("%x", this.mac.Sum(nil))
	}

	if this.metadata.HMAC != hmac {
		fmt.Fprintf(os.Stderr, "Crap. HMAC does not match... :(\nWanted:\t%s\nGot:\t%s\n", this.metadata.HMAC, hmac)
		return this.metadata, E_HMAC_MISMATCH
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
		this.metadata.FileName,
		this.metadata.Uuid,
		strings.ToUpper(this.metadata.Encryption),
		strings.ToUpper(this.metadata.Authentication),
		this.metadata.TotalSize,
		this.metadata.TotalSizeIn, // Taken from metadata, because this has to mach or the HMAC wouldn't
		this.metadata.Chunks,
	)

	return this.metadata, err
}