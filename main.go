package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"runtime"

	"./GoogleDrive"
	"github.com/hashicorp/vault/api"
	"github.com/nightlyone/lockfile"
)

var (
	upload         = flag.String("upload", "", "Filename to upload from stdin")
	list           = flag.Bool("list", false, "List files available")
	chain          = flag.Bool("chain", false, "Display chain of snapshots to restore (can take some time for large datasets)")
	download       = flag.String("download", "", "UUID to download to stdout")
	authentication = flag.String("authentication", "HMAC-SHA3-512", "Define the authentication to use (NONE, HMAC-SHA[3-]{256,512})")
	encryption     = flag.String("encryption", "AES-CTR", "Define the encryption to use (NONE, AES-{CTR,OFB,CFB})")
	folder         = flag.String("folder", "OZB", "Folder on Google Drive to backup to/from")
	passphrase     = flag.String("passphrase", "", "Passphrase to use to en-/decrypt and for authentication")
	quota          = flag.Bool("quota", false, "Define to see Google Drive quota used before continuing")
	chunksize      = flag.Int("chunksize", 256, "Chunksize for files in MiB. Note: You need this space on disk/RAM during up- & download!")
	backup         = flag.String("backup", "", "Specify 'btrfs' or 'zfs' to backup a snapshot")
	restore        = flag.String("restore", "", "Specify 'btrfs' or 'zfs' to restore a snapshot")
	restoreTarget  = flag.String("restoretarget", "", "Specify a zfs/btrfs subvolume to restore to")
	subvolume      = flag.String("subvolume", "", "Subvolume to backup/restore to (btrfs/zfs only)")
	latest         = flag.Bool("latest", false, "Grab latest successfully uploaded snapshot for --subvolume")
	vault          = flag.String("vault", "", "Vault URL to connect to")
	vaultToken     = flag.String("vaulttoken", "", "Vault token to fetch Google Drive secrets with")
	tmpdir         = flag.String("tmpdir", "", "Temporary folder. Default if empty: /dev/shm (in-memory) or os.TempDir if unavailable")
)

func main() {
	flag.Parse()
	runtime.GOMAXPROCS(runtime.NumCPU())

	if *vaultToken != "" {
		vaultConfig := api.Config{Address: *vault}
		vaultClient, err := api.NewClient(&vaultConfig)
		if err != nil {
			panic(err)
		}
		vaultClient.SetToken(*vaultToken)

		GoogleDrive.InitGoogleDrive(vaultClient)
	} else {
		GoogleDrive.InitGoogleDrive(nil)
	}

	//for _, pair := range os.Environ() {
	//	fmt.Println(pair)
	//}

	if *quota {
		GoogleDrive.DisplayQuota()
	}

	if *backup != "" {
		lock, err := lockfile.New("/var/lock/" + base64.StdEncoding.EncodeToString([]byte(*subvolume)) + ".lock")
		if err != nil {
			fmt.Printf("Cannot init lock. reason: %v", err)
			panic(err) // handle properly please!
		}

		err = lock.TryLock()

		// Error handling is essential, as we only try to get the lock.
		if err != nil {
			fmt.Printf("Cannot lock %q, reason: %v", lock, err)
			panic(err) // handle properly please!
		}

		defer lock.Unlock()
	}

	switch {
	case *list:
		parent := GoogleDrive.FindOrCreateFolder(*folder)
		GoogleDrive.ListFiles(parent)
		os.Exit(0)
	case *chain:
		chainCommand()
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
