package main

import (
	"os"
	"strings"

	"./Abstractions"
	"./Btrfs"
	"./Common"
	"./Discard"
	"./ZFS"
	"./GoogleDrive"
	"github.com/dustin/go-humanize"
	"github.com/prometheus/common/log"
)

func chainCommand() {
	if *subvolume == "" {
		log.Fatalln("Must specify --subvolume")
	}
	if *folder == "" {
		log.Fatalln("Must specify --folder")
	}

	folderId := GoogleDrive.FindOrCreateFolder(*folder)
	chain := GoogleDrive.BuildChain(folderId, *subvolume, true)
	printInfo(&chain)
}

func printInfo (chain *[]Common.SnapshotWithSize) {
	if chain == nil {
		return
	}
	var sizeOnDisk uint64 = 0
	var downloadSize uint64 = 0
	for _, snap := range *chain {
		sizeOnDisk += snap.DiskSize
		downloadSize += snap.DownloadSize
	}

	log.Infof("Subvolume: %s", *subvolume)
	log.Infof("Snapshots %d", len(*chain))
	log.Infof("Size on Disk: %s", humanize.IBytes(sizeOnDisk))
	log.Infof("Size to Download: %s", humanize.IBytes(downloadSize))
}

func restoreCommand() {
	if *subvolume == "" {
		log.Fatalln("Must specify --subvolume")
	}
	if *folder == "" {
		log.Fatalln("Must specify --folder")
	}
	if *restoreTarget == "" {
		log.Fatalln("Must specify --restoretarget")
	}

	var manager Common.SnapshotManager

	restoreType := strings.ToLower(*restore)
	switch restoreType {
	case "btrfs":
		manager = Btrfs.NewManager(*folder)
	case "zfs":
		manager = ZFS.NewManager(*folder)
	case "discard":
		manager = Discard.NewManager(*folder)
	default:
		log.Fatalln("--restore only supports btrfs, discard and zfs.")
		os.Exit(1)
	}

	log.Infoln(manager.ListLocalSnapshots())

	var previous string

	log.Info("Building restore chain. This might take a while...")
	folderId := GoogleDrive.FindOrCreateFolder(*folder)
	restoreChain := GoogleDrive.BuildChain(folderId, *subvolume, true)
	printInfo(&restoreChain)
	log.Info("starting restore...")

	for _, snap := range restoreChain {
		wp := &Abstractions.WriteProxy{}
		downloader, err := Abstractions.NewDownloader(wp, *folder, snap.Uuid, *passphrase, *tmpdir)
		if err != nil {
			if err == Abstractions.E_NO_DATA {
				log.Infoln("Snapshot has no data, skipping...")
				continue
			}
		}
		wc, err := manager.Restore(*restoreTarget)
		Common.PrintAndExitOnError(err, 1)
		wp.Proxified = wc
		meta, err := downloader.Download()
		if err != nil {
			log.Fatalf("Restore failed. Error while downloading snapshot: %+v", err)
		}
		log.Infoln(meta, err)

		if previous != "" {
			manager.DeleteSnapshot(previous)
		}
		previous = snap.Filename
	}
}