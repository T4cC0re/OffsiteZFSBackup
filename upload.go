package main

import (
	"./GoogleDrive"
	"fmt"
	"github.com/pierrec/lz4"
	"github.com/satori/go.uuid"
	"io"
	"os"
)

func uploadCommand() {
	parent := GoogleDrive.FindOrCreateFolder(*folder)

	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		settings := StorageSettings{Encryption: *encryption, Authentication: *authentication}
		id, _ := uuid.NewV4()

		fmt.Fprintf(os.Stderr, "Uploading file encrypted with %s and authenticated by %s. UUID: %s\n", settings.Encryption, settings.Authentication, id.String())

		filename := generateFileBaseName(*filename, id.String(), settings, true)
		uploader, err := GoogleDrive.NewGoogleDriveWriter(filename, id.String(), parent, *chunksize * 1024 * 1024)
		if err != nil {
			panic(err)
		}
		defer uploader.Close()

		compress := lz4.NewWriter(uploader)
		compress.Header = lz4.Header{
			BlockDependency: true,
			BlockChecksum:   false,
			NoChecksum:      false,
			BlockMaxSize:    4 << 20,
			HighCompression: false,
		}
		defer compress.Close()

		multiwriter := io.MultiWriter(compress)
		_, err = io.Copy(multiwriter, os.Stdin)
		if err != nil {
			panic(err)
		}
		err = compress.Close()
		if err != nil {
			panic(err)
		}
		err = uploader.Close()
		if err != nil {
			panic(err)
		}

		defer func() {
			fmt.Fprint(os.Stderr, "TODO: Write metadata file and create HMAC!\n")
		}()
	}
}
