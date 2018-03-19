package GoogleDrive

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"path/filepath"

	"crypto/md5"
	"github.com/dustin/go-humanize"
	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"io"
	"time"
)

var (
	E_NOPARENT              = errors.New("no parent found")
	E_BACKEND_HASH_MISMATCH = errors.New("hash of remote file differs from local file")
)

func Upload(name string, uuid string, parent string, reader io.Reader, opt_wantedMD5 string) (*drive.File, error) {
	parents := make([]string, 1)
	parents[0] = parent
	properties := make(map[string]string)
	properties["OZB_uuid"] = uuid
	properties["OZB"] = "true"
	file, err := srv.Files.Create(&drive.File{Name: name, Parents: parents, AppProperties: properties}).Media(reader).Do()
	if err != nil {
		return nil, err
	}

	// If we pass an empty hash, skip verification
	if opt_wantedMD5 == "" {
		return file, nil
	}

	googleDriveMD5 := ""

	for googleDriveMD5 == "" {
		// Re-fetch file, to have hashes and stuff
		fileUpdate, err := srv.Files.Get(file.Id).Fields("md5Checksum, id").Do()
		if err != nil {
			return nil, err
		}

		googleDriveMD5 = fileUpdate.Md5Checksum

		if googleDriveMD5 == "" {
			time.Sleep(5 * time.Second)
		}
	}

	if googleDriveMD5 != opt_wantedMD5 {
		return nil, E_BACKEND_HASH_MISMATCH
	}

	return file, err
}

func Download(fileId string, opt_wantedMD5 string, writer *os.File) (int64, error) {
	_, err := writer.Seek(0, 0)
	if err != nil {
		return 0, err
	}
	err = writer.Truncate(0)
	if err != nil {
		return 0, err
	}

	res, err := srv.Files.
		Get(fileId).
		//AcknowledgeAbuse(true).
		Download()
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()

	hash := md5.New()
	multiWriter := io.MultiWriter(writer, hash)

	n, err := io.Copy(multiWriter, res.Body)
	if err != nil {
		return 0, err
	}

	writtenMD5 := fmt.Sprintf("%x", hash.Sum(nil))

	// Empty opt_wantedMD5 = disable verification
	if opt_wantedMD5 != "" && writtenMD5 != opt_wantedMD5 {
		return 0, E_BACKEND_HASH_MISMATCH
	}

	err = writer.Sync()
	if err != nil {
		return 0, err
	}

	_, err = writer.Seek(0, 0)
	if err != nil {
		return 0, err
	}

	return n, err
}

type ParentFilter struct {
	Parent       string
	WantedParent string
	Qualifying   []string
}

func findId(wanted string, parentID string) (string, error) {
	p := ParentFilter{Parent: "", WantedParent: wanted}
	fileList, err := srv.Files.List().Fields("nextPageToken, files(id, name, parents)").Q("name = '" + p.WantedParent + "' AND '" + parentID + "' in parents AND trashed = false").Do()
	if err != nil {
		return "", err
	}
	for _, file := range fileList.Files {
		return file.Id, nil
	}

	return "", E_NOPARENT
}

func findInFolder(parentID string) (*drive.FileList, error) {
	return srv.Files.
		List().
		Fields("nextPageToken, files").
		Q("'" + parentID + "' in parents AND trashed = false AND appProperties has { key='OZB' and value='true'}").
		Do()
}

// getClient uses a Context and Config to retrieve a Token
// then generate a Client. It returns the generated Client.
func getClient(ctx context.Context, config *oauth2.Config) *http.Client {
	cacheFile, err := tokenCacheFile()
	if err != nil {
		log.Fatalf("Unable to get path to cached credential file. %v", err)
	}
	tok, err := tokenFromFile(cacheFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(cacheFile, tok)
	}
	return config.Client(ctx, tok)
}

// getTokenFromWeb uses Config to request a Token.
// It returns the retrieved Token.
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	var code string
	if _, err := fmt.Scan(&code); err != nil {
		log.Fatalf("Unable to read authorization code %v", err)
	}

	tok, err := config.Exchange(context.Background(), code)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web %v", err)
	}
	return tok
}

// tokenCacheFile generates credential file path/filename.
// It returns the generated credential path/filename.
func tokenCacheFile() (string, error) {
	usr, err := user.Current()
	if err != nil {
		return "", err
	}
	tokenCacheDir := filepath.Join(usr.HomeDir, ".credentials")
	os.MkdirAll(tokenCacheDir, 0700)
	return filepath.Join(tokenCacheDir,
		url.QueryEscape("offsite-zfs-backup.json")), err
}

// tokenFromFile retrieves a Token from a given file path.
// It returns the retrieved Token and any read error encountered.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	t := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(t)
	defer f.Close()
	return t, err
}

// saveToken uses a file path to create a file and store the
// token in it.
func saveToken(file string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", file)
	f, err := os.OpenFile(file, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

var srv *drive.Service

func InitGoogleDrive() {
	ctx := context.Background()

	b, err := ioutil.ReadFile("client_secret.json")
	if err != nil {
		log.Fatalf("Unable to read client secret file: %v", err)
	}

	config, err := google.ConfigFromJSON(b, drive.DriveFileScope)
	if err != nil {
		log.Fatalf("Unable to parse client secret file to config: %v", err)
	}
	client := getClient(ctx, config)

	srv, err = drive.New(client)
	if err != nil {
		log.Fatalf("Unable to retrieve drive Client %v", err)
	}
}

type Quota struct {
	Limit     uint64
	Used      uint64
	Unlimited bool
}

func getQuota() (*Quota, error) {
	q := Quota{}
	resp, err := srv.About.Get().Fields("storageQuota").Do()
	if err != nil {
		return nil, err
	}

	if resp.StorageQuota.Limit == 0 {
		q.Unlimited = true
	}

	q.Limit = uint64(resp.StorageQuota.Limit)
	q.Used = uint64(resp.StorageQuota.Usage)

	return &q, nil
}

func createFolder(name string) (string, error) {
	f := drive.File{Name: name, MimeType: "application/vnd.google-apps.folder"}
	res, err := srv.Files.Create(&f).Fields("id").Do()
	return res.Id, err
}

func ListFiles(parent string) {
	files, err := findInFolder(parent)
	if err != nil {
		panic(err)
	}

	for _, file := range files.Files {
		fmt.Fprintf(os.Stderr, "ITEM: %s\tMD5: %s\tSize: %d (%s)\tID: %s\n", file.Name, file.Md5Checksum, file.Size, humanize.IBytes(uint64(file.Size)), file.Id)
		fmt.Fprintf(os.Stderr, file.Properties["OZB_uuid"])
	}
}

func FindOrCreateFolder(name string) string {
	parent, err := findId(name, "root")
	if err != nil {
		if err == E_NOPARENT {
			parent, err = createFolder(name)
			if err != nil {
				panic(err)
			}
		}
	}
	return parent
}
func DisplayQuota() {
	q, err := getQuota()
	if err != nil {
		panic(err)
	}

	var limit string
	if q.Unlimited {
		limit = "unlimited"
	} else {
		limit = humanize.IBytes(q.Limit)
	}
	fmt.Fprintf(os.Stderr, "Limit: %s, Used: %s\n", limit, humanize.IBytes(q.Used))
}
