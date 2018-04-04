package main

import (
	"./GoogleDrive"
	"flag"
	"fmt"
	"os"
	"runtime"
)

var (
	upload         = flag.String("upload", "", "Filename to upload from stdin")
	list           = flag.Bool("list", false, "List files available")
	download       = flag.String("download", "", "UUID to download to stdout")
	authentication = flag.String("authentication", "HMAC-SHA3-512", "Define the authentication to use (NONE, HMAC-SHA[3-]{256,512})")
	encryption     = flag.String("encryption", "AES-CTR", "Define the encryption to use (NONE, AES-{CTR,OFB,CFB})")
	folder         = flag.String("folder", "OZB", "Folder on Google Drive to backup to/from")
	passphrase     = flag.String("passphrase", "", "Passphrase to use to en-/decrypt and for authentication")
	quota          = flag.Bool("quota", false, "Define to see Google Drive quota used before continuing")
	chunksize      = flag.Int("chunksize", 256, "Chunksize for files in MiB. Note: You need this space on disk during up- & download!")
	backup         = flag.String("backup", "", "Specify 'btrfs' or 'zfs' to backup a snapshot")
	restore        = flag.String("restore", "", "Specify 'btrfs' or 'zfs' to restore a snapshot")
	restoreTarget  = flag.String("restoretarger", "", "Specivy a zfs/btrfs subvolume to restore to")
	subvolume      = flag.String("subvolume", "", "Subvolume to backup/restore to (btrfs/zfs only)")
	latest         = flag.Bool("latest", false, "Grab latest successfully uploaded snapshot for --subvolume")
)

func main() {
	flag.Parse()
	runtime.GOMAXPROCS(runtime.NumCPU())
	GoogleDrive.InitGoogleDrive()

	//for _, pair := range os.Environ() {
	//	fmt.Println(pair)
	//}

	if *quota {
		GoogleDrive.DisplayQuota()
	}

	switch {
	case *list:
		parent := GoogleDrive.FindOrCreateFolder(*folder)
		GoogleDrive.ListFiles(parent)
		os.Exit(0)
	case *backup != "":
		backupCommand()
	case *restore != "":
		restoreCommand()
	case *download != "":
		downloadCommand()
	case *upload != "":
		uploadCommand()
	case *latest:
		if *subvolume == "" {
			fmt.Fprintln(os.Stderr, "Must specify --subvolume")
			os.Exit(1)
		}
		parent := GoogleDrive.FindOrCreateFolder(*folder)
		snapshot, err := GoogleDrive.FetchLatest(parent, *subvolume)
		fmt.Println(snapshot)
		fmt.Fprintln(os.Stderr, err)
	case *quota:
		// NOOP
	default:
		fmt.Fprintln(os.Stderr, "Please select an option")
		os.Exit(1)
	}
}
