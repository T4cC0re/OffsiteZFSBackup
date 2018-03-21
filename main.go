package main

import (
	"./GoogleDrive"
	"flag"
	"fmt"
	"os"
	"runtime"
)

var (
	upload         = flag.Bool("upload", false, "Define to upload from stdin")
	filename       = flag.String("filename", "", "Filename to upload/download")
	list           = flag.Bool("list", false, "List files available")
	download       = flag.Bool("download", false, "Define to download to stdout")
	authentication = flag.String("authentication", "HMAC-SHA3-512", "Define the authentication to use (NONE, HMAC-SHA[3-]{256,512})")
	encryption     = flag.String("encryption", "AES-CTR", "Define the encryption to use (NONE, AES-{CTR,OFB,CFB})")
	folder         = flag.String("folder", "OZB", "Folder on Google Drive to backup to/from")
	passphrase     = flag.String("passphrase", "", "Passphrase to use to en-/decrypt and for authentication")
	quota          = flag.Bool("quota", false, "Define to see Google Drive quota used before continuing")
	chunksize      = flag.Int("chunksize", 256, "Chunksize for files in MiB. Note: You need this space on disk during up- & download!")
)

func main() {
	flag.Parse()
	runtime.GOMAXPROCS(runtime.NumCPU())
	GoogleDrive.InitGoogleDrive()

	if *quota {
		GoogleDrive.DisplayQuota()
	}

	switch {
	case *list:
		parent := GoogleDrive.FindOrCreateFolder(*folder)
		GoogleDrive.ListFiles(parent)
		os.Exit(0)
	case *download:
		if *filename == "" {
			fmt.Fprintln(os.Stderr, "Please set a filename")
			os.Exit(1)
		}
		downloadCommand()
	case *upload:
		if *filename == "" {
			fmt.Fprintln(os.Stderr, "Please set a filename")
			os.Exit(1)
		}
		uploadCommand()
	case *quota:
		// NOOP
	default:
		fmt.Fprintln(os.Stderr, "Please select an option")
		os.Exit(1)
	}
}
