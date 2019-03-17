package main

import (
	"encoding/base64"
	"flag"
	"gitlab.com/T4cC0re/OffsiteZFSBackup/Backend"
	"os"
	"regexp"
	"runtime"
	"strings"

	"fmt"
	"github.com/hashicorp/vault/api"
	"github.com/nightlyone/lockfile"
	log "github.com/sirupsen/logrus"
	"gitlab.com/T4cC0re/OffsiteZFSBackup/Backend/GoogleDrive"
)

var (
	upload         = flag.String("upload", "", "Filename to upload from stdin")
	list           = flag.Bool("list", false, "List files available")
	chain          = flag.Bool("chain", false, "Display chain of snapshots to restore (can take some time for large datasets)")
	download       = flag.String("download", "", "UUID to download to stdout")
	authentication = flag.String("authentication", "HMAC-SHA3-512", "Define the authentication to use (NONE, HMAC-SHA[3-]{256,512})")
	encryption     = flag.String("encryption", "AES-CTR", "Define the encryption to use (NONE, AES-{CTR,OFB,CFB})")
	folder         = flag.String("folder", "", "Folder to backup to/from (with prefix)")
	passphrase     = flag.String("passphrase", "", "Passphrase to use to en-/decrypt and for authentication")
	quota          = flag.Bool("quota", false, "Define to see Google Drive quota used before continuing")
	chunksize      = flag.Int("chunksize", 256, "Chunksize for files in MiB. Note: You need this space on disk/RAM during up- & download!")
	backup         = flag.String("backup", "", "Specify 'btrfs' or 'zfs' to backup a snapshot")
	restore        = flag.String("restore", "", "Specify 'btrfs' or 'zfs' to restore a snapshot")
	restoreTarget  = flag.String("restoretarget", "", "Specify a zfs/btrfs subvolume to restore to")
	subvolume      = flag.String("subvolume", "", "Subvolume to backup/restore to (btrfs/zfs only)")
	latest         = flag.Bool("latest", false, "Grab latest successfully uploaded snapshot for --subvolume")
	vault          = flag.String("vault", "", "Vault URL to connect to (overrules 'VAULT_ADDR')")
	vaultToken     = flag.String("vaulttoken", "", "Vault token to fetch Google Drive secrets with (overrules 'VAULT_TOKEN')")
	tmpdir         = flag.String("tmpdir", "", "Temporary folder. Default if empty: /dev/shm (in-memory) or os.TempDir if unavailable")
	full           = flag.Bool("full", false, "Force a full backup instead of doing an incemental one")
	cleanup        = flag.Bool("cleanup", false, "Remove unneeded snapshots and delete inaddressable files from Google Drive at the end. If specified without --backup only Google Drive will be cleaned up")
	ratio        = flag.Int("ratio", 3, "lz4 compression ratio to use")
)

var backend  Backend.Backend
var glue *Backend.Glue

func isGDrive() bool {
	if strings.HasPrefix(*folder, "gdrive:") {
		return true
	}
	return false
}

func isSSH() bool {
	if match, _ := regexp.MatchString(`(?mi)^([^@]+)@([^:]+):(.*)$`, *folder); match {
		return true
	}
	return false
}

func isLocal() bool {
	if !isGDrive() && !isSSH() {
		return true
	}
	return false
}

func initBackend(client *api.Client) {
	var err error
	switch {
	case isGDrive():
		backend, err = GoogleDrive.Init(client)
		if err != nil {
			log.WithField("error", err).Fatal()
		}
	case isSSH():
	case isLocal():
		fallthrough
	default:

	}
	if backend != nil {
		glue = Backend.CreateGlue(&backend)
	}
	log.
		WithField("backend", fmt.Sprintf("%+v", backend)).
		WithField("glue", fmt.Sprintf("%+v", glue)).Info()
}

func main() {
	flag.Parse()
	runtime.GOMAXPROCS(runtime.NumCPU())

	log.WithField("isSSH", isSSH()).WithField("isGDrive", isGDrive()).WithField("isLocal", isLocal()).Info()

	vaultAddrEnv := os.Getenv("VAULT_ADDR")
	vaultTokenEnv := os.Getenv("VAULT_TOKEN")

	if vaultAddrEnv != "" && *vault == "" {
		*vault = vaultAddrEnv
	}
	if vaultTokenEnv != "" && *vaultToken == "" {
		*vaultToken = vaultTokenEnv
	}

	if *vaultToken != "" {
		log.Infoln("Using vault to access secrets...")
		vaultConfig := api.Config{Address: *vault}
		vaultClient, err := api.NewClient(&vaultConfig)
		if err != nil {
			log.Errorln(err)
			// Try a regular init as a fail-safe
			initBackend(nil)
		}
		vaultClient.SetToken(*vaultToken)
		initBackend(nil)
	} else {
		initBackend(nil)
	}

	//TODO: NEW HACK FOR NOW!
	var drive GoogleDrive.GoogleDrive
	var isDrive bool
	if drive, isDrive = backend.(GoogleDrive.GoogleDrive); isDrive {
		log.Infoln("Detected GoogleDrive")
	} else {
		log.Fatalln("NO GDrive")
	}
	// END HACK

	if *quota {
		if isGDrive() {
			glue.DisplayQuota()
		} else {
			log.WithField("error", "Quota not supported for SCP backend").Error()
		}
	}

	if *backup != "" {
		lock, err := lockfile.New("/var/lock/" + base64.StdEncoding.EncodeToString([]byte(*subvolume)) + ".lock")
		if err != nil {
			log.Fatalf("Cannot init lock. reason: %v", err)
		}

		err = lock.TryLock()

		// Error handling is essential, as we only try to get the lock.
		if err != nil {
			log.Fatalf("Cannot lock %q, reason: %v", lock, err)
		}

		defer lock.Unlock()
	}

	switch {
	case *list:
		if isGDrive() {
			drive.ListFiles(*folder)
		} else {
			log.WithField("error", "List not supported for SCP backend").Error()
		}
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
			log.Fatalln("Must specify --subvolume")
		}
		if *folder == "" {
			log.Fatalln("Must specify --folder")
		}
		snapshot, err := drive.FetchLatest(*folder, *subvolume)
		fmt.Println(snapshot, err)
	case *quota:
		// NOOP
	case *cleanup:
		// Cleanup without a backup.

		if *subvolume == "" {
			log.Fatalln("Must specify --subvolume")
		}
		if *folder == "" {
			log.Fatalln("Must specify --folder")
		}
		drive.Cleanup(*folder, *subvolume)
	default:
		log.Fatalln("Please select an option")
	}
}
