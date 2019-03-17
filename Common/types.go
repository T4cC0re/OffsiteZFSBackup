package Common

type MetadataBase struct {
	Uuid           string
	FileName       string
	Encryption     string
	Authentication string
	IsData         bool
}

type Metadata struct {
	Uuid           string
	FileName       string
	Encryption     string
	Authentication string
	HMAC           string
	IV             string
	TotalSizeIn    uint64
	TotalSize      uint64
	Chunks         uint64
	FileType       string
	Subvolume      string
	Date           int64
	Parent         string
}

type ChunkInfo struct {
	Uuid           string
	FileName       string
	Encryption     string
	Authentication string
	IsData         bool
	Chunk          uint64
}
