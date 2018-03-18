package GoogleDrive

import (
	"errors"
	"fmt"
	"github.com/dustin/go-humanize"
	"golang.org/x/net/context"
	"google.golang.org/api/drive/v3"
	"io"
	"io/ioutil"
	"os"
	"regexp"
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
	cache     *os.File
	chunkPos  int64
	Total     int64
	chunk     uint
	uuid      string
	parentID  string
	closed    bool
	fileIDs   map[uint]string
	chunkSize map[uint]int64
	hitEOF    bool
}

var chunkInfoRegexp = regexp.MustCompile(`(?mi)^([a-z0-9]{8}-[a-z0-9]{4}-4[a-z0-9]{3}-[89ab][a-z0-9]{3}-[a-z0-9]{12})\|([^|]+)\|([^|]+)\|([^|]+)\|(D|M)\|(\d+)$`)

func parseFileName(filename string) (*ChunkInfo, error) {
	matches := chunkInfoRegexp.FindStringSubmatch(filename)
	if matches == nil || len(matches) != 7 {
		return nil, E_CHUNKINFO
	}
	//TODO: Read this from appProperties, but keep something like this for recovery of those attributes
	chunkInfo := ChunkInfo{}
	chunkInfo.Uuid = matches[1]
	chunkInfo.FileName = matches[2]
	chunkInfo.Encryption = matches[3]
	chunkInfo.Authentication = matches[4]
	chunkInfo.IsData = matches[5] == "D"
	chunk, err := strconv.ParseUint(matches[6], 10, 32)
	if err != nil {
		return nil, err
	}
	chunkInfo.Chunk = uint(chunk)

	return &chunkInfo, nil
}

type ChunkInfo struct {
	Uuid           string
	FileName       string
	Encryption     string
	Authentication string
	IsData         bool
	Chunk          uint
}

func NewGoogleDriveReader(uuid string, parentID string) (*Reader, error) {
	cache, err := ioutil.TempFile("", READ_CACHE_FILENAME)
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
	reader := &Reader{cache: cache, chunkPos: 0, chunk: 0, uuid: uuid, parentID: parentID, closed: false, chunkSize: make(map[uint]int64), fileIDs: make(map[uint]string), hitEOF: false}

	err = srv.Files.
		List().
		Fields("nextPageToken, files").
		Q("appProperties has { key='OZB_uuid' and value='"+uuid+"' }").
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
	if len(reader.fileIDs) != int(maxIndex+1) {
		return nil, E_CHUNKS_MISSING
	}

	reader.download(0)

	return reader, nil
}

func (this *Reader) gatherChunkInfo(fileList *drive.FileList) error {
	for _, file := range fileList.Files {
		chunkInfo, err := parseFileName(file.Name)
		if err != nil {
			return err
		}

		this.fileIDs[chunkInfo.Chunk] = file.Id
		this.chunkSize[chunkInfo.Chunk] = file.Size
	}

	return nil
}

func (this *Reader) download(chunk uint) error {
	fmt.Fprintf(os.Stderr, "\033[2KDownloading chunk %d for a total of %s...\r", this.chunk, humanize.IBytes(uint64(this.Total)+uint64(this.chunkPos)))
	for {
		size, err := Download(this.fileIDs[chunk], this.cache)
		fmt.Println(size, this.chunkSize[chunk], err)
		if err != nil && size != this.chunkSize[chunk] {
			fmt.Fprintf(os.Stderr, "\033[2KDownload of chunk %d failed for a total of %s Retrying...\r", this.chunk, humanize.IBytes(uint64(this.Total)+uint64(this.chunkPos)))
			time.Sleep(time.Microsecond * 250)
			continue
		}

		this.Total += int64(this.chunkPos)
		this.chunkPos = 0
		fmt.Fprintf(os.Stderr, "\033[2KDownloaded chunk %d for a total of %s.\n", this.chunk, humanize.IBytes(uint64(this.Total)))
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

		lastChunk := int(this.chunk + 1) == len(this.fileIDs)
		if lastChunk {
			copy(p, read1)
			this.hitEOF = true
			return len(read1), nil
		}

		err := this.download(this.chunk + 1)
		if err != nil {
			return 0, err
		}
		this.chunk++
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
		return int(availableToRead + restToRead), nil
	} else {
		buff := make([]byte, len(p))
		currentLocation, err := this.readIt(buff)
		if err != nil {
			return 0, err
		}
		this.chunkPos = currentLocation

		copy(p, buff)

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

	return nil
}
