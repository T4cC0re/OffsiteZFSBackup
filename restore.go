package main

import (
	"fmt"
	"os"
	"strings"

	"./Abstractions"
	"./Btrfs"
	"./Common"
	"./GoogleDrive"
	"./ZFS"
	"github.com/dustin/go-humanize"
)

func chainCommand() {
	if *subvolume == "" {
		fmt.Fprintln(os.Stderr, "Must specify --subvolume")
		os.Exit(1)
	}

	var sizeOnDisk uint64 = 0
	var downloadSize uint64 = 0

	chain := buildChain(false)
	for _, snap := range chain {
		sizeOnDisk += snap.DiskSize
		downloadSize += snap.DownloadSize
	}

	fmt.Fprintf(os.Stderr, "%s\n\t- %d Snapshots\n\t- Size on Disk: %s\n\t- Size to Download: %s\n", *subvolume, len(chain), humanize.IBytes(sizeOnDisk), humanize.IBytes(downloadSize))
}

func restoreCommand() {
	if *subvolume == "" {
		fmt.Fprintln(os.Stderr, "Must specify --subvolume")
		os.Exit(1)
	}
	if *restoreTarget == "" {
		fmt.Fprintln(os.Stderr, "Must specify --restoretarget")
		os.Exit(1)
	}

	var manager Common.SnapshotManager

	restoreType := strings.ToLower(*restore)
	switch restoreType {
	case "btrfs":
		manager = Btrfs.NewManager(*folder)
	case "zfs":
		manager = ZFS.NewManager(*folder)
	default:
		fmt.Fprintln(os.Stderr, "--restore only supports btrfs and zfs.")
		os.Exit(1)
	}

	fmt.Fprintln(os.Stderr, manager.ListLocalSnapshots())

	var previous string

	for _, snap := range buildChain(false) {
		local := manager.IsAvailableLocally(snap.Filename)
		fmt.Fprintf(os.Stderr, "%s - exists?: %v\n", snap.Filename, local)

		//if local == true {
		//	break
		//}

		wp := &Abstractions.WriteProxy{}
		downloader, err := Abstractions.NewDownloader(wp, *folder, snap.Uuid, *passphrase, *tmpdir)
		if err != nil {
			if err == Abstractions.E_NO_DATA {
				fmt.Fprintln(os.Stderr, "Snapshot has no data, skipping...")
				continue
			}
		}
		wc, err := manager.Restore(*restoreTarget)
		Common.PrintAndExitOnError(err, 1)
		wp.Proxified = wc
		meta, err := downloader.Download()
		fmt.Fprintln(os.Stderr, meta, err)

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
		snap := Common.SnapshotWithSize{Uuid: latestUuid, Filename: latestName, DownloadSize:downloadSize, DiskSize: diskSize}
		if print {
			fmt.Fprintf(os.Stderr, "snapshot:\n\t- UUID: %s\n\t- Name: %s\n\t- Parent: %s\n", latestUuid, latestName, latestParent)
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
