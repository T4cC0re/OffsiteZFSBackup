package Btrfs

import (
	"../Common"
	"../GoogleDrive"
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
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

func (this *Manager) ListLocalSnapshots() []string {
	cmd := exec.Command("btrfs", "subvolume", "list", "-ros", snapshotdir)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		log.Fatal(err)
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

func (this *Manager) CreateSnapshot(subvolume string) string {
	snapshotname := fmt.Sprintf(
		"%s/%s@%d",
		snapshotdir,
		base64.RawURLEncoding.EncodeToString([]byte(subvolume)),
		time.Now().Unix(),
	)
	cmd := exec.Command("btrfs", "subvolume", "snapshot", "-r", subvolume, snapshotname)

	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return ""
	}

	return snapshotname
}

func (this *Manager) Stream(snapshot string, parentSnapshot string) (io.ReadCloser, error) {
	var command *exec.Cmd
	if parentSnapshot == "" {
		command = exec.Command("btrfs", "send", snapshot)
	} else {
		command = exec.Command("btrfs", "send", "-p", parentSnapshot, snapshot)
	}

	rc, err := command.StdoutPipe()
	command.Stderr = os.Stderr
	if err != nil {
		return nil, err
	}
	err = command.Start()
	if err != nil {
		log.Fatal(err)
	}

	return rc, nil
}

func (this *Manager) Restore(targetSubvolume string) (io.WriteCloser, error) {
	fmt.Fprintln(os.Stderr, "NOT IMPLEMENTED, YET!")
	return nil, nil

	/// ZFS implementation below
	command := exec.Command("zfs", "receive", targetSubvolume)

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
