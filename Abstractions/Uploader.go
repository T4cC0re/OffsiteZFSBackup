package Abstractions

import (
	"../Common"
	"../GoogleDrive"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"github.com/pierrec/lz4"
	"github.com/satori/go.uuid"
	"hash"
	"io"
	"os"
	"strings"
	"time"
)

type Uploader struct {
	inputMeta   *GoogleDrive.MetadataBase
	multiWriter io.Writer
	readProxy   *ReadProxy
	compress    *lz4.Writer
	mac         hash.Hash
	keyStream   cipher.Stream
	uploader    *GoogleDrive.Writer
	timestamp   int64
	iv          []byte
	parent      string
	fileType    string
	subvolume   string
	Parent      string
}

func NewUploader(r io.ReadCloser, fileType string, subvolume string, folder string, filename string, passphrase string, encryption string, authentication string, chunksize int) *Uploader {
	this := &Uploader{}

	this.fileType = fileType
	this.subvolume = subvolume

	this.timestamp = time.Now().Unix()

	var writers []io.Writer

	this.parent = GoogleDrive.FindOrCreateFolder(folder)

	id, _ := uuid.NewV4()

	var writeTarget io.Writer

	this.iv = make([]byte, aes.BlockSize)
	n, err := rand.Read(this.iv)
	if n != aes.BlockSize || err != nil {
		panic(errors.New("failed to generate IV"))
	}

	encryptionL := strings.ToLower(encryption)
	authenticationL := strings.ToLower(authentication)

	this.inputMeta = &GoogleDrive.MetadataBase{Uuid: id.String(), FileName: filename, IsData: true, Authentication: authenticationL, Encryption: encryptionL}

	this.uploader, err = GoogleDrive.NewGoogleDriveWriter(this.inputMeta, this.parent, chunksize*1024*1024)
	if err != nil {
		panic(err)
	}

	this.mac, this.keyStream = Common.PrepareMACAndEncryption(passphrase, this.iv, this.inputMeta.Authentication, this.inputMeta.Encryption, false)
	if this.mac != nil {
		writers = append(writers, this.mac)
	}
	if this.keyStream == nil {
		writeTarget = this.uploader
	} else {
		writeTarget = cipher.StreamWriter{S: this.keyStream, W: this.uploader, Err: nil}
	}

	this.compress = lz4.NewWriter(writeTarget)
	this.compress.Header = lz4.Header{
		BlockDependency: true,
		BlockChecksum:   false,
		NoChecksum:      false,
		BlockMaxSize:    4 << 20,
		HighCompression: false,
	}

	writers = append(writers, this.compress)

	this.readProxy = &ReadProxy{r, 0}

	this.multiWriter = io.MultiWriter(writers...)

	return this
}

func (this *Uploader) close() (error, error) {
	err := this.compress.Close()
	err2 := this.uploader.Close()
	return err, err2
}

func (this *Uploader) Upload() (*GoogleDrive.Metadata, error) {
	fmt.Fprintf(os.Stderr, "Uploading as '%s'\n", this.inputMeta.Uuid)

	// Here the actual reading and upload begins
	_, err := io.Copy(this.multiWriter, this.readProxy)
	if err != nil {
		return nil, err
	}

	err, err2 := this.close()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	if err2 != nil {
		fmt.Fprintln(os.Stderr, err2)
	}

	var authHMAC string
	if this.mac != nil {
		authHMAC = fmt.Sprintf("%x", this.mac.Sum(nil))
	}

	meta := &GoogleDrive.Metadata{
		HMAC:           authHMAC,
		IV:             fmt.Sprintf("%x", this.iv),
		FileName:       this.inputMeta.FileName,
		Uuid:           this.inputMeta.Uuid,
		Authentication: this.inputMeta.Authentication,
		Encryption:     this.inputMeta.Encryption,
		TotalSizeIn:    this.readProxy.Total,
		TotalSize:      this.uploader.Total,
		Chunks:         this.uploader.Chunk,
		FileType:       this.fileType,
		Subvolume:      this.subvolume,
		Date:           this.timestamp,
		Parent:         this.Parent,
	}

	//Print summary:
	fmt.Fprintf(
		os.Stderr,
		"\nSummary:\n"+
			" - Filename: '%s'\n"+
			" - UUID: '%s'\n"+
			" - Crypto: %s with %s\n"+
			" - Bytes read: %d\n"+
			" - Bytes uploaded: %d (lz4 compressed)\n"+
			" - Chunks: %d\n",
		meta.FileName,
		meta.Uuid,
		strings.ToUpper(meta.Encryption),
		strings.ToUpper(meta.Authentication),
		meta.TotalSizeIn,
		meta.TotalSize,
		meta.Chunks,
	)

	for {
		fmt.Fprint(os.Stderr, "\033[2KUploading metadata...\r")
		if GoogleDrive.UploadMetadata(meta, this.parent) != nil {
			fmt.Fprint(os.Stderr, "\033[2KMetadata uploaded\n")
			break
		}
		fmt.Fprint(os.Stderr, "\033[2KFailed to upload metadata. Retrying...\r")
		time.Sleep(5 * time.Second)
	}

	return meta, err
}
