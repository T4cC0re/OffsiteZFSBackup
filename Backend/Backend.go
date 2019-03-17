package Backend

import (
	"github.com/dustin/go-humanize"
	log "github.com/sirupsen/logrus"
	"gitlab.com/T4cC0re/OffsiteZFSBackup/Common"
)

type Backend interface {
	Quota() (usedBytes int64, totalBytes int64)
	UploadMetadata(meta *Common.Metadata, path string) *Common.Metadata
}

type Glue struct {
	backend *Backend
}

type Abstract struct {
	Backend
}

func CreateGlue(backend *Backend) *Glue {
	g := &Glue{backend}
	return g
}

func (b Glue) DisplayQuota() {
	used, total := (*b.backend).Quota()
	limit := humanize.IBytes(uint64(total))
	if used == -1 {
		limit = "unlimited"
	}
	log.Infof("Limit: %s, Used: %s", limit, humanize.IBytes(uint64(used)))
}
