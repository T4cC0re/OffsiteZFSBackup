package GoogleDrive

import (
	"errors"
	"github.com/dustin/go-humanize"
	log "github.com/sirupsen/logrus"
	"gitlab.com/T4cC0re/OffsiteZFSBackup/Common"
	"golang.org/x/net/context"
	"google.golang.org/api/drive/v3"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"time"
)

const READ_CACHE_FILENAME = "OZBReadCache"

var E_READER_CLOSED = errors.New("reader closed")
var E_CHUNKINFO = errors.New("chunkinfo could not be parsed")
var E_CHUNKS_MISSING = errors.New("some chunks are missing")
var E_READ_TOO_SHORT = errors.New("data read from cache smaller than expected")

type Reader struct {
	io.Reader
	drive 	  *GoogleDrive
	cache     *os.File
	chunkPos  int64
	Total     int64
	chunk     uint
	uuid      string
	closed    bool
	fileIDs   map[uint]string
	fileMD5s  map[uint]string
	chunkSize map[uint]int64
	hitEOF    bool
}

func NewGoogleDriveReader(drive *GoogleDrive, meta *Common.Metadata, tmpBase string) (*Reader, error) {
	if tmpBase == "" {
		stat, err := os.Stat("/dev/shm")
		if err == nil && stat.IsDir() {
			tmpBase = "/dev/shm"
			log.Infoln( "Using shared memory as cache...")
		}
	}

	err := os.MkdirAll(tmpBase, 0777)
	if err != nil {
		return nil, err
	}

	cache, err := ioutil.TempFile(tmpBase, READ_CACHE_FILENAME)
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
	reader := &Reader{drive: drive, cache: cache, chunkPos: 0, chunk: 0, uuid: meta.Uuid, closed: false, chunkSize: make(map[uint]int64), fileIDs: make(map[uint]string), fileMD5s: make(map[uint]string), hitEOF: false}

	// TODO: Limit fields to fetch!
	err = reader.drive.srv.Files.
		List().
		Fields("nextPageToken, files").
		Q("properties has { key='OZB_uuid' and value='"+meta.Uuid+"' } AND properties has { key='OZB_type' and value='data' }").
		Pages(context.Background(), reader.gatherChunkInfo)

	if err != nil {
		return nil, err
	}

	// Make sure we got all chunks available
	var maxIndex uint = 0
	for index := range reader.fileIDs {
		if index > maxIndex {
			maxIndex = index
		}
	}
	if len(reader.fileIDs) != int(maxIndex+1) || uint64(len(reader.fileIDs)) != meta.Chunks {
		return nil, E_CHUNKS_MISSING
	}

	reader.download(0)
	log.Infof("Reading from chunk %d...", 0)

	return reader, nil
}

func (this *Reader) gatherChunkInfo(fileList *drive.FileList) error {
	for _, file := range fileList.Files {
		raw, err := strconv.ParseUint(file.Properties["OZB_chunk"], 10, 32)
		if err != nil {
			return err
		}

		chunkId := uint(raw)

		this.fileIDs[chunkId] = file.Id
		this.fileMD5s[chunkId] = file.Md5Checksum
		this.chunkSize[chunkId] = file.Size
	}

	return nil
}

func (this *Reader) download(chunk uint) error {
	for {
		log.Infof( "Downloading chunk %d...", chunk)
		size, err := this.drive.Download(this.fileIDs[chunk], this.fileMD5s[chunk], this.cache)
		if err != nil {
			log.Errorf("Download of chunk %d failed. %s Retrying...", chunk, err.Error())
			time.Sleep(5 * time.Second)
			continue
		}

		this.chunkPos = 0
		log.Infof("Downloaded chunk %d (%s)", chunk, humanize.IBytes(uint64(size)))
		break
	}

	return nil
}

func (this *Reader) readIt(p []byte) (int64, error) {
	n, err := this.cache.Read(p)
	if err != nil {
		return int64(this.chunkPos), err
	}
	if n != len(p) {
		return 0, E_READ_TOO_SHORT
	}

	curloc, err := this.cache.Seek(0, 1)
	if err != nil {
		return int64(this.chunkPos), err
	}

	return curloc, nil
}

func (this *Reader) Read(p []byte) (int, error) {
	if this.closed {
		return 0, E_READER_CLOSED
	}
	if this.hitEOF {
		return 0, io.EOF

	}

	availableToRead := this.chunkSize[this.chunk] - this.chunkPos
	wantToRead := int64(len(p))

	if wantToRead > availableToRead {
		read1 := make([]byte, availableToRead)
		if availableToRead != 0 {
			currentLocation, err := this.readIt(read1)
			if err != nil {
				return 0, err
			}
			this.chunkPos = currentLocation
		}
		restToRead := wantToRead - availableToRead

		lastChunk := int(this.chunk+1) == len(this.fileIDs)
		if lastChunk {
			copy(p, read1)
			this.hitEOF = true
			this.Total += int64(len(read1))
			return len(read1), nil
		}

		err := this.download(this.chunk + 1)
		if err != nil {
			return 0, err
		}
		this.chunk++

		log.Infof("Reading from chunk %d...", this.chunk)

		if this.chunkSize[this.chunk] < restToRead {
			restToRead = this.chunkSize[this.chunk]
		}
		read2 := make([]byte, restToRead)
		currentLocation, err := this.readIt(read2)
		if err != nil {
			return 0, err
		}
		this.chunkPos = currentLocation

		wholeRead := append(read1, read2...)
		copy(p, wholeRead)
		this.Total += availableToRead + restToRead
		return int(availableToRead + restToRead), nil
	} else {
		buff := make([]byte, len(p))
		currentLocation, err := this.readIt(buff)
		if err != nil {
			return 0, err
		}
		this.chunkPos = currentLocation

		copy(p, buff)

		this.Total += int64(len(p))
		return len(p), nil
	}
}

func (this *Reader) Close() error {
	if this.closed {
		return nil
	} // Ignore double closes

	err := this.cache.Close()
	if err != nil {
		return err
	}

	err = os.Remove(this.cache.Name())
	if err != nil {
		return err
	}

	log.Infof("Finished stream after %s", humanize.IBytes(uint64(this.Total)))

	return nil
}
