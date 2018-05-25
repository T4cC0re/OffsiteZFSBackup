package Common

type Snapshot struct {
	Uuid     string
	Filename string
}

type SnapshotWithSize struct {
	Uuid         string
	Filename     string
	DownloadSize uint64
	DiskSize     uint64
}
