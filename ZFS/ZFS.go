package ZFS

import (
	"bytes"
	"fmt"
	log "github.com/sirupsen/logrus"
	"gitlab.com/T4cC0re/OffsiteZFSBackup/Backend"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"gitlab.com/T4cC0re/OffsiteZFSBackup/Backend/GoogleDrive"
	"gitlab.com/T4cC0re/OffsiteZFSBackup/Common"
)

type Manager struct {
	Common.SnapshotManager
	parent string
}

func NewManager(folder string, backend *Backend.Backend) *Manager {
	this := &Manager{}

	//TODO: NEW HACK FOR NOW!
	var drive GoogleDrive.GoogleDrive
	var isDrive bool
	b2 := *backend
	if drive, isDrive = b2.(GoogleDrive.GoogleDrive); isDrive {
		log.Infoln("Detected GoogleDrive")
	} else {
		log.Fatalln("NO GDrive")
	}
	// END HACK

	this.parent = drive.FindOrCreateFolder(folder)
	return this
}

func  (this *Manager) Cleanup(subvolume string, latestSnapshot string) () {
	log.Infof("ZFS Cleanup...")
	snapshotsToDelete := []string{}
	log.Infof(subvolume)

	snapshotPattern := fmt.Sprintf("%s@", subvolume)

	snaps := this.ListLocalSnapshots()

	for _, snap := range snaps {
		if snap == latestSnapshot {
			// NEVER delete the latest snapshots, because the would not allow for incremental backups
			continue
		}
		if strings.HasPrefix(snap, snapshotPattern) {
			snapshotsToDelete = append(snapshotsToDelete, snap)
		}
	}

	log.Infof("Deleting snapshots: %+v", snapshotsToDelete)
	for _, snap := range snapshotsToDelete {
		this.DeleteSnapshot(snap)
	}
}

func (this *Manager) ListLocalSnapshots() []string {
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
		if snap != "" {
			snapshots = append(snapshots, snap)
		}
	}

	return snapshots
}

func (this *Manager) IsAvailableLocally(snapshot string) bool {
	for _, snap := range this.ListLocalSnapshots() {
		if snapshot == snap {
			return true
		}
	}
	return false
}

func (this *Manager) CreateSnapshot(subvolume string) (string, error) {
	snapshotname := fmt.Sprintf(
		"%s@%d",
		subvolume,
		time.Now().Unix(),
	)
	cmd := exec.Command("zfs", "snapshot", snapshotname)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return "", err
	}

	cmd = exec.Command("zfs", "hold", "keep", snapshotname)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		return "", err
	}

	return strings.TrimLeft(snapshotname, "/"), nil
}

func (this *Manager) DeleteSnapshot(snapshot string) (bool, error) {
	if !strings.Contains(snapshot, "@") {
		return false, Common.E_INVALID_SNAPSHOT
	}

	cmd := exec.Command("zfs", "release", "-r", "keep", snapshot)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return false, err
	}

	cmd = exec.Command("zfs", "destroy", snapshot)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		return false, err
	}

	return true, nil
}

func (this *Manager) Stream(snapshot string, parentSnapshot string) (io.ReadCloser, error) {
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

func (this *Manager) Restore(targetSubvolume string) (io.WriteCloser, error) {
	command := exec.Command("zfs", "receive", "-F", targetSubvolume)

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
