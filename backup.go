package main

import (
	"github.com/prometheus/common/log"
	"gitlab.com/T4cC0re/OffsiteZFSBackup/Abstractions"
	"gitlab.com/T4cC0re/OffsiteZFSBackup/Backend/GoogleDrive"
	"gitlab.com/T4cC0re/OffsiteZFSBackup/Btrfs"
	"gitlab.com/T4cC0re/OffsiteZFSBackup/Common"
	"gitlab.com/T4cC0re/OffsiteZFSBackup/ZFS"
	gdrive "google.golang.org/api/drive/v3"
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
		manager = Btrfs.NewManager(*folder, &backend)
	case "zfs":
		manager = ZFS.NewManager(*folder, &backend)
	default:
		log.Fatalln("--backup only supports btrfs and zfs.")
	}

	// TODO: HACK HACK HACK
	if drive, isDrive := (backend).(GoogleDrive.GoogleDrive); isDrive {
		log.Infoln("Detected GoogleDrive")

		var latestUploaded *gdrive.File
		var err error
		if !*full {
			latestUploaded, err = drive.FindLatest(*folder, *subvolume)
			log.Error(err)
		}

		var parentSnapshotUuid string
		var parentSnapshotName string
		if latestUploaded != nil {
			if !manager.IsAvailableLocally(latestUploaded.Properties["OZB_filename"]) {
				log.Fatalf("Latest uploaded snapshot '%s' is not available locally! Backup with --full to create a new full backup.", latestUploaded.Properties["OZB_filename"])
			}
			parentSnapshotUuid = latestUploaded.Properties["OZB_uuid"]
			parentSnapshotName = latestUploaded.Properties["OZB_filename"]
		} else {
			log.Infof("Doing full backup, as no uploaded snapshot was found or --full was specified.")
		}
		if *cleanup {
			log.Infof("Will clean up after backup...")
		}

		currentSnapshot, err := manager.CreateSnapshot(*subvolume)
		if err != nil {
			log.Fatalln("Failed to create snapshot", err.Error())
		}

		log.Infof("Latest uploaded snapshot:")
		log.Infof("UUID: %s", parentSnapshotUuid)
		log.Infof("Name: %s", parentSnapshotName)

		/// No Wait necessary, as command will EOF which will finish the upload
		rc, err := manager.Stream(currentSnapshot, parentSnapshotName)
		Common.PrintAndExitOnError(err, 1)

		uploader := Abstractions.NewUploader(&backend, rc, backupType, *subvolume, *folder, currentSnapshot, *passphrase, *encryption, *authentication, *chunksize, *tmpdir, *ratio)
		if latestUploaded != nil {
			uploader.Parent = parentSnapshotUuid
		}
		meta, err := uploader.Upload()
		Common.PrintAndExitOnError(err, 1)

		fileId, err := drive.SaveLatest(currentSnapshot, meta.Uuid, *subvolume, *folder)
		Common.PrintAndExitOnError(err, 1)

		log.Infof("FileID of state: %s", fileId)

		if *cleanup {
			log.Infof("Cleaning up...")
			manager.Cleanup(*subvolume, currentSnapshot)
			drive.Cleanup(*folder, *subvolume)
		}
	}
}
