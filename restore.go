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
)

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

	for _, snap := range buildChain() {
		local := manager.IsAvailableLocally(snap.Filename)
		fmt.Fprintf(os.Stderr, "%s - exists?: %v\n", snap.Filename, local)

		//if local == true {
		//	break
		//}

		wp := Abstractions.WriteProxy{}
		downloader, err := Abstractions.NewDownloader(wp, *folder, snap.Uuid, *passphrase)
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
	}
}

func buildChain() []Common.Snapshot {
	latestUploaded, err := GoogleDrive.FindLatest(GoogleDrive.FindOrCreateFolder(*folder), *subvolume)
	Common.PrintAndExitOnError(err, 1)

	if latestUploaded == nil {
		return []Common.Snapshot{}
	}

	latestUuid := latestUploaded.Properties["OZB_uuid"]
	latestName := latestUploaded.Properties["OZB_filename"]

	var chain []Common.Snapshot

	for true {
		fs, _ := GoogleDrive.FetchMetadata(latestUuid, GoogleDrive.FindOrCreateFolder(*folder))
		latestUuid = fs.Uuid
		latestName = fs.FileName
		latestParent := fs.Parent
		snap := Common.Snapshot{Uuid: latestUuid, Filename: latestName}
		fmt.Fprintf(os.Stderr, "snapshot:\n\t- UUID: %s\n\t- Name: %s\n\t- Parent: %s\n", latestUuid, latestName, latestParent)
		chain = append([]Common.Snapshot{snap}, chain...)
		if latestParent == "" {
			break
		}

		// fetch parent on next iteration
		latestUuid = latestParent
	}

	return chain
}
