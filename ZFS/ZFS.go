package ZFS

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"../Common"
	"../GoogleDrive"
)

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
	fmt.Fprintln(os.Stderr, "NOT TESTED, YET!")
	cmd := exec.Command("zfs", "list", "-Ht", "snapshot")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		log.Fatal(err)
	}

	var snapshots []string

	for _, snapshot := range strings.Split(out.String(), "\n") {
		snap := strings.Split(snapshot, "\t")[0]
		fmt.Fprintln(os.Stderr, snap)
		if snap != "" {
			snapshots = append(snapshots, snap)
		}
	}

	fmt.Fprintln(os.Stderr, snapshots)
	return snapshots
}

func (this *Manager) IsAvailableLocally(snapshot string) bool {
	fmt.Fprintln(os.Stderr, "NOT TESTED, YET!")
	for _, snap := range this.ListLocalSnapshots() {
		if snapshot == snap {
			return true
		}
	}
	return false
}

func (this *Manager) CreateSnapshot(subvolume string) string {
	fmt.Fprintln(os.Stderr, "NOT TESTED, YET!")
	snapshotname := fmt.Sprintf(
		"%s@%d",
		subvolume,
		time.Now().Unix(),
	)
	cmd := exec.Command("zfs", "snapshot", snapshotname)

	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return ""
	}

	return strings.TrimLeft(snapshotname, "/")
}

func (this *Manager) Stream(snapshot string, parentSnapshot string) (io.ReadCloser, error) {
	fmt.Fprintln(os.Stderr, "NOT TESTED, YET!")

	var command *exec.Cmd
	if parentSnapshot == "" {
		command = exec.Command("zfs", "send", snapshot)
	} else {
		command = exec.Command("zfs", "send", "-i", parentSnapshot, snapshot)
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
