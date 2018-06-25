package GoogleDrive

import (
	"crypto/md5"
	//"encoding/json"
	"errors"
	"fmt"
	"github.com/dustin/go-humanize"
	"hash"
	"io"
	"io/ioutil"
	"os"
	"time"
)

const WRITE_CACHE_FILENAME = "OZBWriteCache"

var E_WRITER_CLOSED = errors.New("writer closed")

// PassThru wraps an existing io.Reader.
//
// It simply forwards the Read() call, while displaying
// the results from individual calls to it.
type Writer struct {
	io.WriteCloser
	cache        *os.File
	written      int
	Total        uint64
	cacheSize    int
	Chunk        uint
	fileNameBase string
	parentID     string
	closed       bool
	meta         *MetadataBase
	hash         hash.Hash
}

func NewGoogleDriveWriter(meta *MetadataBase, parentID string, cacheSize int, tmpBase string) (*Writer, error) {
	if tmpBase == "" {
		stat, err := os.Stat("/dev/shm")
		if err == nil && stat.IsDir() {
			tmpBase = "/dev/shm"
			fmt.Fprintln(os.Stderr, "Using shared memory as cache...")
		}
	}

	err := os.MkdirAll(tmpBase, 0777)
	if err != nil {
		return nil, err
	}

	cache, err := ioutil.TempFile(tmpBase, WRITE_CACHE_FILENAME)
	if err != nil {
		return nil, err
	}

	_, err = cache.Seek(0, 0)
	if err != nil {
		return nil, err
	}

	err = cache.Truncate(0)
	if err != nil {
		return nil, err
	}

	writer := &Writer{cache: cache, written: 0, Chunk: 0, parentID: parentID, cacheSize: cacheSize, closed: false, meta: meta, hash: md5.New()}

	return writer, nil
}

func (this *Writer) upload() error {
	err := this.cache.Sync()
	if err != nil {
		return err
	}

	fileHash := fmt.Sprintf("%x", this.hash.Sum(nil))

	chunkInfo := &ChunkInfo{Uuid: this.meta.Uuid, Encryption: this.meta.Encryption, Authentication: this.meta.Authentication, IsData: true, FileName: this.meta.FileName, Chunk: this.Chunk}
	for {
		fmt.Fprintf(os.Stderr, "\033[2KUploading chunk %d for a total of %s...\r", this.Chunk, humanize.IBytes(uint64(this.Total)+uint64(this.written)))
		_, err = this.cache.Seek(0, 0)
		if err != nil {
			return err
		}

		driveFile, err := Upload(chunkInfo, this.parentID, this.cache, fileHash)
		if err != nil {
			fmt.Fprintf(os.Stderr, "\033[2KUpload of chunk %d failed for a total of %s Retrying...\r", this.Chunk, humanize.IBytes(uint64(this.Total)+uint64(this.written)))
			time.Sleep(5 * time.Second)
			continue
		}

		this.Total += uint64(this.written)
		this.written = 0
		fmt.Fprintf(os.Stderr, "\033[2KUploaded chunk %d for a total of %s. ID: %s\n", this.Chunk, humanize.IBytes(uint64(this.Total)), driveFile.Id)
		this.Chunk++
		break
	}

	_, err = this.cache.Seek(0, 0)
	if err != nil {
		return err
	}
	err = this.cache.Truncate(0)
	if err != nil {
		return err
	}

	this.hash = md5.New()

	return nil
}

func (this *Writer) writeSync(p []byte) (int64, error) {
	n, err := this.cache.Write(p)
	if err != nil {
		return int64(this.written), err
	}

	this.hash.Write(p)

	err = this.cache.Sync()
	if err != nil {
		return int64(this.written), err
	}

	this.written += n
	curloc, err := this.cache.Seek(0, 1)
	if err != nil {
		return int64(this.written), err
	}

	return curloc, nil
}

func (this *Writer) Write(p []byte) (int, error) {
	if this.closed {
		return 0, E_WRITER_CLOSED
	}

	if this.written == 0 {
		fmt.Fprintf(os.Stderr, "\033[2KWriting into chunk %d...\r", this.Chunk)
	}

	buff := make([]byte, len(p))
	copy(buff, p)

	for (len(buff) + this.written) > this.cacheSize {
		toWrite := this.cacheSize - this.written

		// Move <toWrite> bytes from buff to smallBuff
		smallBuff := make([]byte, toWrite)
		copy(smallBuff, buff)
		buff = append([]byte(nil), buff[toWrite:]...)

		currentLocation, err := this.writeSync(smallBuff)
		if err != nil {
			return 0, err
		}

		if currentLocation >= int64(this.cacheSize) {
			err = this.upload()
		}
	}

	currentLocation, err := this.writeSync(buff)
	if err != nil {
		return 0, err
	}

	if currentLocation >= int64(this.cacheSize) {
		err = this.upload()
	}

	return len(p), nil
}

func (this *Writer) Close() error {
	if this.closed {
		return nil
	} // Ignore double closes

	err := this.upload()
	if err != nil {
		return err
	}

	_ = this.cache.Close()

	err = os.Remove(this.cache.Name())
	if err != nil {
		return err
	}

	return nil
}
