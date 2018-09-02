package Btrfs

import (
	"../Common"
	"../GoogleDrive"
	"bytes"
	"fmt"
	"hash/crc32"
	"io"
	"github.com/prometheus/common/log"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
	"path"
)

/*
Valid:
ID 267 gen 358 cgen 357 top level 5 otime 2018-03-22 22:09:00 path var/backups/snapshots/root@1521752940
ID 267 gen 358 cgen 357 top level 5 otime 2018-03-22 22:09:00 path var/backups/snapshots/root@1521752940|root@1521752945
*/
var btrfsSnapshotRegExp = regexp.MustCompile(`([^\s]+?)$`)

var snapshotdir = "/var/backups/snapshots"

type Manager struct {
	Common.SnapshotManager
	parent string
}

func NewManager(folder string) *Manager {
	this := &Manager{}
	this.parent = GoogleDrive.FindOrCreateFolder(folder)
	return this
}

func (this *Manager) Cleanup(subvolume string, latestSnapshot string) () {
	log.Infof("btrfs Cleanup...")
	snapshotsToDelete := []string{}
	volume := strconv.FormatUint(uint64(crc32.ChecksumIEEE([]byte(subvolume))), 16)
	log.Infof(volume, subvolume)

	snapshotPattern := fmt.Sprintf("%s/%s@", snapshotdir, volume)

	snaps := this.ListLocalSnapshots()

	for _, snap := range snaps {
		if snap == latestSnapshot {
			// NEVER delete the latest snapshots, because the would not allow for incremental backups
			continue
		}
		if strings.Contains(snap, snapshotPattern) {
			snapshotsToDelete = append(snapshotsToDelete, snap)
		}
	}

	log.Infof("Deleting snapshots: %+v", snapshotsToDelete)
	for _, snap := range snapshotsToDelete {
		this.DeleteSnapshot(snap)
	}
}

func (this *Manager) ListLocalSnapshots() []string {
	cmd := exec.Command("btrfs", "subvolume", "list", "-ros", snapshotdir)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		_, statErr := os.Stat(snapshotdir)
		// Does not exist. Create the subvolume
		os.MkdirAll(path.Base(snapshotdir), 0777)
		if statErr != os.ErrNotExist {
			cmd = exec.Command("btrfs", "subvolume", "create", snapshotdir)
			var out bytes.Buffer
			cmd.Stdout = &out
			err = cmd.Run()
			if err != nil {
				log.Fatalf("Could not create btrfs subvolume '%s': %v", snapshotdir, err)
			}
			return []string{}
		}
		log.Fatal(statErr)
	}

	var snapshots []string

	for _, snapshot := range strings.Split(out.String(), "\n") {
		snap := ParseBtrfsSnapshot(snapshot)
		if snap != "" {
			snapshots = append(snapshots, snap)
		}
	}

	return snapshots
}
func (this *Manager) IsAvailableLocally(snapshot string) bool {
	for _, snap := range this.ListLocalSnapshots() {
		if snap == snapshot {
			return true
		}
	}
	return false
}

func ParseBtrfsSnapshot(snapshot string) string {
	matches := btrfsSnapshotRegExp.FindStringSubmatch(snapshot)
	if matches == nil || len(matches) != 2 {
		return ""
	}

	return "/" + matches[1]
}

func (this *Manager) CreateSnapshot(subvolume string) (string, error) {
	snapshotname := fmt.Sprintf(
		"%s/%s@%d",
		snapshotdir,
		strconv.FormatUint(uint64(crc32.ChecksumIEEE([]byte(subvolume))), 16),
		time.Now().Unix(),
	)
	cmd := exec.Command("btrfs", "subvolume", "snapshot", "-r", subvolume, snapshotname)

	var out bytes.Buffer
	cmd.Stdout = &out
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", err
	}
	log.Info(strings.Trim(out.String(), "\n\r"))
	log.Error(strings.Trim(stderr.String(), "\n\r"))

	return snapshotname, nil
}

func (this *Manager) DeleteSnapshot(snapshot string) (bool, error) {
	if !strings.Contains(snapshot, "@") {
		return false, Common.E_INVALID_SNAPSHOT
	}

	cmd := exec.Command("btrfs", "subvolume", "delete", snapshot)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return false, err
	}

	return true, nil
}

func (this *Manager) Stream(snapshot string, parentSnapshot string) (io.ReadCloser, error) {
	var command *exec.Cmd
	if parentSnapshot == "" {
		command = exec.Command("btrfs", "send", snapshot)
	} else {
		command = exec.Command("btrfs", "send", "-p", parentSnapshot, snapshot)
	}

	rc, err := command.StdoutPipe()
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err != nil {
		return nil, err
	}
	err = command.Start()
	if err != nil {
		log.Fatal(err)
	}
	log.Error(stderr.String())

	return rc, nil
}

func (this *Manager) Restore(targetSubvolume string) (io.WriteCloser, error) {
	os.MkdirAll(targetSubvolume, 0644)
	command := exec.Command("btrfs", "receive", targetSubvolume)

	wc, err := command.StdinPipe()
	command.Stderr = os.Stderr
	if err != nil {
		return nil, err
	}
	err = command.Start()
	if err != nil {
		log.Fatal(err)
	}

	return wc, nil
}
