package main

import (
	"os"
	"strings"

	"./Abstractions"
	"./Btrfs"
	"./Common"
	"./Discard"
	"./GoogleDrive"
	"./ZFS"
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

	var sizeOnDisk uint64 = 0
	var downloadSize uint64 = 0

	chain := buildChain(false)
	for _, snap := range chain {
		sizeOnDisk += snap.DiskSize
		downloadSize += snap.DownloadSize
	}

	log.Infof("Subvolume: %s", *subvolume)
	log.Infof("Snapshots %d", len(chain))
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

	for _, snap := range buildChain(false) {
		local := manager.IsAvailableLocally(snap.Filename)
		log.Infof("%s - exists?: %v", snap.Filename, local)

		//if local == true {
		//	break
		//}

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
		log.Infoln(meta, err)

		if previous != "" {
			manager.DeleteSnapshot(previous)
		}
		previous = snap.Filename
	}
}

func buildChain(print bool) []Common.SnapshotWithSize {
	folderId := GoogleDrive.FindOrCreateFolder(*folder)
	latestUploaded, err := GoogleDrive.FindLatest(folderId, *subvolume)
	Common.PrintAndExitOnError(err, 1)

	if latestUploaded == nil {
		return []Common.SnapshotWithSize{}
	}

	latestUuid := latestUploaded.Properties["OZB_uuid"]
	latestName := latestUploaded.Properties["OZB_filename"]

	var chain []Common.SnapshotWithSize

	for true {
		fs, err := GoogleDrive.FetchMetadata(latestUuid, folderId)
		if err != nil {
			panic(err)
		}
		latestUuid = fs.Uuid
		latestName = fs.FileName
		latestParent := fs.Parent
		downloadSize := fs.TotalSize
		diskSize := fs.TotalSizeIn
		snap := Common.SnapshotWithSize{Uuid: latestUuid, Filename: latestName, DownloadSize: downloadSize, DiskSize: diskSize}
		if print {
			log.Infof("snapshot:")
			log.Infof("UUID: %s", latestUuid)
			log.Infof("Name: %s", latestName)
			log.Infof("Parent: %s", latestParent)
		}
		chain = append([]Common.SnapshotWithSize{snap}, chain...)
		if latestParent == "" {
			break
		}

		// fetch parent on next iteration
		latestUuid = latestParent
	}

	return chain
}
