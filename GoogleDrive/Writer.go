package GoogleDrive

import (
	"errors"
	"fmt"
	"github.com/dustin/go-humanize"
	"io"
	"io/ioutil"
	"os"
	"time"
)

const CACHE_FILENAME = "OZBCache"

var E_WRITER_CLOSED = errors.New("writer closed")

// PassThru wraps an existing io.Reader.
//
// It simply forwards the Read() call, while displaying
// the results from individual calls to it.
type Writer struct {
	io.Writer
	cache        *os.File
	written      int
	Total        int64
	cacheSize    int
	chunk        int
	fileNameBase string
	parentID     string
	closed       bool
}

func NewGoogleDriveWriter(fileNameBase string, parentID string, cacheSize int) (*Writer, error) {
	cache, err := ioutil.TempFile("", CACHE_FILENAME)
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

	writer := &Writer{cache: cache, written: 0, chunk: 0, fileNameBase: fileNameBase, parentID: parentID, cacheSize: cacheSize, closed: false}
	return writer, nil
}

func (this *Writer) upload() error {
	err := this.cache.Sync()
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "\033[2KUploading chunk %d for a total of %s...\r", this.chunk, humanize.IBytes(uint64(this.Total)+uint64(this.written)))
	for {
		_, err = this.cache.Seek(0, 0)
		if err != nil {
			return err
		}

		driveFile, err := Upload(fmt.Sprintf("%s|%08d", this.fileNameBase, this.chunk), this.parentID, this.cache)
		if err != nil {
			fmt.Fprintf(os.Stderr, "\033[2KUpload of chunk %d failed for a total of %s Retrying...\r", this.chunk, humanize.IBytes(uint64(this.Total)+uint64(this.written)))
			time.Sleep(time.Microsecond * 250)
			continue
		}

		this.Total += int64(this.written)
		this.written = 0
		fmt.Fprintf(os.Stderr, "\033[2KUploaded chunk %d for a total of %s. ID: %s\n", this.chunk, humanize.IBytes(uint64(this.Total)), driveFile.Id)
		this.chunk++
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
	return nil
}

func (this *Writer) writeSync(p []byte) (int64, error) {
	n, err := this.cache.Write(p)
	if err != nil {
		return int64(this.written), err
	}

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

	buff := make([]byte, len(p))
	copy(buff, p) // Do not alter what was passed

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

	err = this.cache.Close()
	if err != nil {
		return err
	}

	err = os.Remove(this.cache.Name())
	if err != nil {
		return err
	}

	return nil
}
