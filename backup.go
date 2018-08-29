package main

import (
	"./Abstractions"
	"./Btrfs"
	"./Common"
	"./GoogleDrive"
	"./ZFS"
	"github.com/prometheus/common/log"
	"strings"
)

func backupCommand() {
	if *subvolume == "" {
		log.Fatalln("Must specify --subvolume")
	}
	if *folder == "" {
		log.Fatalln("Must specify --folder")
	}

	var manager Common.SnapshotManager

	backupType := strings.ToLower(*backup)
	switch backupType {
	case "btrfs":
		manager = Btrfs.NewManager(*folder)
	case "zfs":
		manager = ZFS.NewManager(*folder)
	default:
		log.Fatalln("--backup only supports btrfs and zfs.")
	}

	latestUploaded, err := GoogleDrive.FindLatest(GoogleDrive.FindOrCreateFolder(*folder), *subvolume)

	manager.ListLocalSnapshots()
	snap, err := manager.CreateSnapshot(*subvolume)
	if err != nil {
		log.Fatalln("Failed to create snapshot", err.Error())
	}

	var parentUuid string
	var parentName string
	if latestUploaded != nil {
		parentUuid = latestUploaded.Properties["OZB_uuid"]
		parentName = latestUploaded.Properties["OZB_filename"]
	}
	log.Infof("Latest uploaded snapshot:")
	log.Infof("UUID: %s", parentUuid)
	log.Infof("Name: %s", parentName)

	/// No Wait necessary, as command will EOF which will finish the upload
	rc, err := manager.Stream(snap, parentName)
	Common.PrintAndExitOnError(err, 1)

	uploader := Abstractions.NewUploader(rc, backupType, *subvolume, *folder, snap, *passphrase, *encryption, *authentication, *chunksize, *tmpdir)
	if latestUploaded != nil {
		uploader.Parent = parentUuid
	}
	meta, err := uploader.Upload()
	Common.PrintAndExitOnError(err, 1)

	fileId, err := GoogleDrive.SaveLatest(snap, meta.Uuid, *subvolume, *folder)
	Common.PrintAndExitOnError(err, 1)

	log.Infof("FileID of state: %s", fileId)

	if parentName != "" {
		_, err = manager.DeleteSnapshot(parentName)
		if err != nil {
			log.Errorf("An error occured while deleting parent snapshot: '%s'", err.Error())
		} else {
			log.Infoln("Succesfully deleted previously parent!")
		}
	} else {
		log.Infoln("no parent to delete")
	}
}
