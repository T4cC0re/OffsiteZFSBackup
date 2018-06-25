package main

import (
	"./Abstractions"
	"./Btrfs"
	"./Common"
	"./GoogleDrive"
	"./ZFS"
	"fmt"
	"os"
	"strings"
)

func backupCommand() {
	if *subvolume == "" {
		fmt.Fprintln(os.Stderr, "Must specify --subvolume")
		os.Exit(1)
	}

	var manager Common.SnapshotManager

	backupType := strings.ToLower(*backup)
	switch backupType {
	case "btrfs":
		manager = Btrfs.NewManager(*folder)
	case "zfs":
		manager = ZFS.NewManager(*folder)
	default:
		fmt.Fprintln(os.Stderr, "--backup only supports btrfs and zfs.")
		os.Exit(1)
	}

	latestUploaded, err := GoogleDrive.FindLatest(GoogleDrive.FindOrCreateFolder(*folder), *subvolume)

	manager.ListLocalSnapshots()
	snap, err := manager.CreateSnapshot(*subvolume)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to create snapshot", err.Error())
		os.Exit(1)
	}

	var parentUuid string
	var parentName string
	if latestUploaded != nil {
		parentUuid = latestUploaded.Properties["OZB_uuid"]
		parentName = latestUploaded.Properties["OZB_filename"]
	}
	fmt.Fprintf(os.Stderr, "Latest uploaded snapshot:\n\t- UUID: %s\n\t- Name: %s\n\t- Parent: %s\n", parentUuid, parentName)

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

	fmt.Fprintf(os.Stderr, "FileID of state: %s\n", fileId)

	if parentName != "" {
		_, err = manager.DeleteSnapshot(parentName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "An error occured while deleting parent snapshot:\n%s\n", err.Error())
		} else {
			fmt.Fprintln(os.Stderr, "Succesfully deleted previously parent!")
		}
	} else {
		fmt.Fprintln(os.Stderr, "no parent to delete")
	}
}
