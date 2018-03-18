package main

import (
	"./GoogleDrive"
	"flag"
	"fmt"
	"os"
	"runtime"
)

type StorageSettings struct {
	Encryption     string
	Authentication string
}

func generateFileBaseName(filename string, uuid string, settings StorageSettings, isData bool) string {
	fileType := "D"
	if !isData {
		fileType = "M"
	}

	return fmt.Sprintf("%s|%s|%s|%s|%s", uuid, filename, settings.Encryption, settings.Authentication, fileType)
}

var (
	upload         = flag.Bool("upload", false, "Define to upload from stdin")
	filename       = flag.String("filename", "", "Filename to upload/download")
	list           = flag.Bool("list", false, "List files available")
	download       = flag.Bool("download", false, "Define to download to stdout")
	authentication = flag.String("authentication", "HMAC-SHA512", "Define the authentication to use")
	encryption     = flag.String("encryption", "NONE", "Define the encryption to use")
	folder         = flag.String("folder", "OZB", "Folder on Google Drive to backup to/from")
	passphrase     = flag.String("passphrase", "", "Passphrase to use to en-/decrypt and for authentication")
	quota          = flag.Bool("quota", false, "Define to see Google Drive quota used (non-exclusive)")
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
		//parent = GoogleDrive.FindOrCreateFolder(*folder)
		fmt.Fprintln(os.Stderr, "Not yet implemented")
		os.Exit(1)
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
