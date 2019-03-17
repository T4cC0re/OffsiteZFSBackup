package Discard

import (
	"io"
	"gitlab.com/T4cC0re/OffsiteZFSBackup/Common"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

var E_STUB = errors.New("stub. Not implemented")

type Manager struct {
	Common.SnapshotManager
	parent string
}

func NewManager(folder string) *Manager {
	this := &Manager{}
	this.parent = ""
	return this
}

func (this *Manager) Cleanup(subvolume string, latestSnapshot string) () {
	log.Infof("noop cleanup...")
	return
}

func (this *Manager) ListLocalSnapshots() []string {
	return []string{}
}

func (this *Manager) IsAvailableLocally(snapshot string) bool {
	return false
}

func (this *Manager) CreateSnapshot(subvolume string) (string, error) {
	return "", E_STUB
}

func (this *Manager) DeleteSnapshot(snapshot string) (bool, error) {
	return false, E_STUB
}

func (this *Manager) Stream(snapshot string, parentSnapshot string) (io.ReadCloser, error) {
	return nil, E_STUB
}

func (this *Manager) Restore(_ string) (io.WriteCloser, error) {
	log.Warn("---- Discarding downloaded data ----")
	return DiscardCloser{}, nil
}
