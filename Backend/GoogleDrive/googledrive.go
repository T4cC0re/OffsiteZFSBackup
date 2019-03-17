package GoogleDrive

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	log "github.com/sirupsen/logrus"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	vault "github.com/hashicorp/vault/api"
	"gitlab.com/T4cC0re/OffsiteZFSBackup/Backend"
	"gitlab.com/T4cC0re/OffsiteZFSBackup/Common"
	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
)

var (
	E_NOPARENT              = errors.New("no parent found")
	E_NO_LATEST             = errors.New("no latest found")
	E_BACKEND_HASH_MISMATCH = errors.New("hash of remote file differs from local file")
)

type GoogleDrive struct {
	Backend.Backend
	srv          *drive.Service
	secretsCache map[string]string
	folderIDs    map[string]string
}

var secretsCache map[string]string

func Init(client *vault.Client) (backend Backend.Backend, err error) {
	g := GoogleDrive{}
	g.secretsCache = make(map[string]string, 0)
	g.folderIDs = make(map[string]string, 0)
	ctx := context.Background()

	var b []byte
	if client != nil {
		err = g.fillSecretsCache(client)
		if err != nil {
			log.Errorf("Could not fill secrets cache from Vault: %v", err)
			b, err = g.getClientSecretFromFile() // fallback
		} else {
			b, err = g.getClientSecretFromVault()
			if err != nil {
				log.Errorf("Could not get client secret from Vault: %v", err)
				b, err = g.getClientSecretFromFile() // fallback
			}
		}
	} else {
		b, err = g.getClientSecretFromFile()
	}

	if err != nil {
		return
	}

	config, err := google.ConfigFromJSON(b, drive.DriveFileScope)
	if err != nil {
		return
	}
	driveClient := g.getClient(ctx, config)

	secretsCache = make(map[string]string, 0)

	backend = g

	g.srv, err = drive.New(driveClient)
	if err != nil {
		return nil, err
	} else {
		return g, nil
	}
}

func (g GoogleDrive) Quota() (usedBytes int64, totalBytes int64) {
	resp, err := g.srv.About.Get().Fields("storageQuota").Do()
	if err != nil {
		return -1, -1
	}

	if resp.StorageQuota.Limit == 0 {
		return resp.StorageQuota.Usage, -1
	}

	return resp.StorageQuota.Usage, resp.StorageQuota.Limit
}

func (g *GoogleDrive) Cleanup(path string, subvolume string) () {
	folderId := g.FindOrCreateFolder(path)

	log.Infof("Google Drive Cleanup...")
	log.Info("Builing restore chain...")
	chain := g.BuildChain(path, subvolume, false)

	log.Info("Retrieving list of files...")
	files, err := g.srv.Files.
		List().
		Fields("nextPageToken, files(id, properties)").
		Q("'" + folderId + "' in parents AND trashed = false").
		Do()
	if err != nil {
		log.Error(err)
	}
	log.Infof("Retrieving %d files", len(files.Files))

	log.Info("Deleting files...")
driveFiles:
	for _, file := range files.Files {
		log.Info(file.Properties["OZB_uuid"], file.Id)
		if file.Properties["OZB_uuid"] == "" || file.Id == "" {
			continue driveFiles // Failsafe
		}
		for _, snap := range chain {
			if snap.Uuid == file.Properties["OZB_uuid"] {
				continue driveFiles
			}
		}
		log.Infof("Deleting %s", file.Id)
		err := g.srv.Files.Delete(file.Id).Do()
		if err != nil {
			log.Error(err)
		}
	}

	log.Infof("Google Drive Cleanup done!")
}

func (g *GoogleDrive) BuildChain(path string, subvolume string, print bool) []Common.SnapshotWithSize {

	latestUploaded, err := g.FindLatest(path, subvolume)
	Common.PrintAndExitOnError(err, 1)

	if latestUploaded == nil {
		return []Common.SnapshotWithSize{}
	}

	latestUuid := latestUploaded.Properties["OZB_uuid"]
	latestName := latestUploaded.Properties["OZB_filename"]

	var chain []Common.SnapshotWithSize

	for true {
		fs, err := g.FetchMetadata(latestUuid, path)
		if err != nil {
			log.Fatal(err)
		}
		latestUuid = fs.Uuid
		latestName = fs.FileName
		latestParent := fs.Parent
		downloadSize := fs.TotalSize
		diskSize := fs.TotalSizeIn
		snap := Common.SnapshotWithSize{Uuid: latestUuid, Filename: latestName, DownloadSize: downloadSize, DiskSize: diskSize}
		if print {
			log.Infof("snapshot: %s", latestName)
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

func (g *GoogleDrive) FetchMetadata(uuid string, path string) (*Common.Metadata, error) {
	parent := g.FindOrCreateFolder(path)

	var query = "trashed = false AND properties has { key='OZB_type' and value='metadata' } AND properties has { key='OZB_uuid' and value='" + uuid + "' }"
	if parent != "" {
		query = "'" + parent + "' in parents AND " + query
	}
	files, err := g.srv.Files.
		List().
		Fields("nextPageToken, files").
		Q(query).
		Do()
	if err != nil {
		return nil, err
	}

	var res *http.Response
	for _, file := range files.Files {
		res, err = g.srv.Files.Get(file.Id).Download()
		break
	}
	if err != nil {
		return nil, err
	}
	if res != nil {
		defer res.Body.Close()
	}

	marshalled, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	unmarshalled := &Common.Metadata{}
	json.Unmarshal(marshalled, unmarshalled)
	if err != nil {
		return nil, err
	}

	return unmarshalled, nil
}

func (g *GoogleDrive) FindLatest(path string, subvolume string) (*drive.File, error) {
	parent := g.FindOrCreateFolder(path)

	files, err := g.srv.Files.
		List().
		Fields("nextPageToken, files").
		Q("'" + parent + "' in parents AND trashed = false AND properties has { key='OZB_type' and value='latest' } AND properties has { key='OZB_subvolume' and value='" + subvolume + "' }").
		Do()
	if err != nil {
		return nil, err
	}

	for _, file := range files.Files {
		return file, nil
	}

	return nil, nil
}

func (g *GoogleDrive) FetchLatest(path string, subvolume string) (string, error) {
	file, err := g.FindLatest(path, subvolume)

	if file.Id == "" {
		return "", E_NO_LATEST
	}

	res, err := g.srv.Files.Get(file.Id).Download()
	defer res.Body.Close()

	latest, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", E_NO_LATEST
	}

	return string(latest), nil
}

func (g GoogleDrive) UploadMetadata(meta *Common.Metadata, path string) *Common.Metadata {
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return nil
	}

	reader := bytes.NewReader(metaBytes)

	parents := make([]string, 1)
	parents[0] = g.FindOrCreateFolder(path)
	properties := make(map[string]string)
	properties["OZB"] = "true"
	properties["OZB_uuid"] = meta.Uuid
	properties["OZB_filename"] = meta.FileName
	properties["OZB_encryption"] = meta.Encryption
	properties["OZB_authentication"] = meta.Authentication
	properties["OZB_chunk"] = fmt.Sprintf("%d", meta.Chunks)
	properties["OZB_storesize"] = fmt.Sprintf("%d", meta.TotalSize)
	properties["OZB_filetype"] = meta.FileType
	properties["OZB_subvolume"] = meta.Subvolume
	properties["OZB_parent"] = meta.Parent
	properties["OZB_date"] = fmt.Sprintf("%d", meta.Date)
	properties["OZB_type"] = "metadata"
	filename := fmt.Sprintf("%s|M", meta.Uuid)
	_, err = g.srv.Files.Create(&drive.File{Name: filename, Parents: parents, Properties: properties}).Media(reader).Do()
	if err != nil {
		return nil
	}

	return meta
}
func (g *GoogleDrive) SaveLatest(snapshotname string, snapshotUUID string, subvolume string, path string) (string, error) {
	reader := bytes.NewReader([]byte(snapshotname))

	properties := make(map[string]string)
	properties["OZB"] = "true"
	properties["OZB_uuid"] = snapshotUUID
	properties["OZB_filename"] = snapshotname
	properties["OZB_chunk"] = "0"
	properties["OZB_storesize"] = fmt.Sprintf("%d", len(snapshotname))
	properties["OZB_filetype"] = "latest"
	properties["OZB_subvolume"] = subvolume
	properties["OZB_date"] = fmt.Sprintf("%d", time.Now().Unix())
	properties["OZB_type"] = "latest"
	filename := fmt.Sprintf("%s|latest", subvolume)

	var file *drive.File

	parent := g.FindOrCreateFolder(path)

	file, err := g.FindLatest(path, subvolume)
	if file == nil {
		var parents []string
		parents = append(parents, parent)
		file, err = g.srv.
			Files.
			Create(&drive.File{Name: filename, Parents: parents, Properties: properties}).
			Media(reader).
			Do()
	} else {
		file, err = g.srv.
			Files.
			Update(file.Id, &drive.File{Name: filename, Properties: properties}).
			Media(reader).
			Do()
	}
	if err != nil {
		return "", err
	}

	return file.Id, nil
}

func (g *GoogleDrive) Upload(meta *Common.ChunkInfo, path string, reader io.Reader, opt_wantedMD5 string) (*drive.File, error) {
	parents := make([]string, 1)
	parents[0] = g.FindOrCreateFolder(path)
	properties := make(map[string]string)
	properties["OZB"] = "true"
	properties["OZB_uuid"] = meta.Uuid
	properties["OZB_chunk"] = fmt.Sprintf("%d", meta.Chunk)
	properties["OZB_type"] = "data"
	filename := fmt.Sprintf("%s|%d", meta.Uuid, meta.Chunk)
	file, err := g.srv.Files.Create(&drive.File{Name: filename, Parents: parents, Properties: properties}).Media(reader).Do()
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
		fileUpdate, err := g.srv.Files.Get(file.Id).Fields("md5Checksum, id").Do()
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

func (g *GoogleDrive) Download(fileId string, opt_wantedMD5 string, writer *os.File) (int64, error) {
	_, err := writer.Seek(0, 0)
	if err != nil {
		return 0, err
	}
	err = writer.Truncate(0)
	if err != nil {
		return 0, err
	}

	res, err := g.srv.Files.
		Get(fileId).
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

func (g *GoogleDrive) findFileIdInParentId(wantedFileName string, parentID string) (string, error) {
	fileList, err := g.srv.
		Files.
		List().
		Fields("nextPageToken, files(id, name, parents)").
		Q("name = '" + wantedFileName + "' AND '" + parentID + "' in parents AND trashed = false").
		Do()
	if err != nil {
		return "", err
	}

	for _, file := range fileList.Files {
		return file.Id, nil
	}

	return "", E_NOPARENT
}

type folderSearch struct {
	files    []*drive.File
	callback func(*drive.File)
}

func (this *folderSearch) add(list *drive.FileList) error {
	if this.callback != nil {
		for _, file := range list.Files {
			this.callback(file)
		}
	}

	this.files = append(this.files, list.Files...)
	return nil
}

func (this *folderSearch) Files() []*drive.File {
	return this.files
}

func (g *GoogleDrive) FindInFolder(path string, fileType string, subvolume string, callback func(*drive.File)) (*folderSearch, error) {
	parentID := g.FindOrCreateFolder(path)

	search := folderSearch{callback: callback}

	query := "'" + parentID + "' in parents AND trashed = false AND properties has { key='OZB_type' and value='metadata' }"

	if fileType != "" {
		query += " AND properties has { key='OZB_filetype' and value='" + fileType + "' }"
	}

	if subvolume != "" {
		query += " AND properties has { key='OZB_subvolume' and value='" + subvolume + "' }"
	}

	err := g.srv.Files.
		List().
		Fields("nextPageToken, files").
		Q(query).
		Pages(context.Background(), search.add)
	if err != nil {
		return nil, err
	}

	return &search, nil
}

// getClient uses a Context and Config to retrieve a Token
// then generate a Client. It returns the generated Client.
func (g *GoogleDrive) getClient(ctx context.Context, config *oauth2.Config) *http.Client {
	cacheFile, err := g.tokenCacheFile()
	if err != nil {
		log.Fatalf("Unable to get path to cached credential file. %v", err)
	}

	var tok *oauth2.Token
	if len(secretsCache) != 0 {
		tok, err = g.tokenFromSecretsCache()
		if err != nil {
			log.Fatal(err)
		}
	} else {
		tok, err = g.tokenFromFile(cacheFile)
		if err != nil {
			tok = g.getTokenFromWeb(config)
			g.saveToken(cacheFile, tok)
		}
	}

	return config.Client(ctx, tok)
}

// getTokenFromWeb uses Config to request a Token.
// It returns the retrieved Token.
func (g *GoogleDrive) getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Fprintf(os.Stderr, "Go to the following link in your browser then type the "+
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
func (g *GoogleDrive) tokenCacheFile() (string, error) {
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
func (g *GoogleDrive) tokenFromSecretsCache() (*oauth2.Token, error) {
	data := secretsCache["offsite-zfs-backup.json"]

	t := &oauth2.Token{}
	err := json.Unmarshal([]byte(data), t)

	return t, err
}

// tokenFromFile retrieves a Token from a given file path.
// It returns the retrieved Token and any read error encountered.
func (g *GoogleDrive) tokenFromFile(file string) (*oauth2.Token, error) {
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
func (g *GoogleDrive) saveToken(file string, token *oauth2.Token) {
	fmt.Fprintf(os.Stderr, "Saving credential file to: %s", file)
	f, err := os.OpenFile(file, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

func (g *GoogleDrive) fillSecretsCache(client *vault.Client) error {
	secret, err := client.Logical().Read("/secret/ozb/googledrive")
	if err != nil {
		return err
	}

	for k, v := range secret.Data {
		lol, ok := v.(string)
		if !ok {
			return errors.New("invalid secret content")
		}
		secretsCache[k] = lol
	}

	if len(secretsCache) != 0 {
		return nil
	}

	return errors.New("invalid secret content (empty)")
}

func (g *GoogleDrive) getClientSecretFromVault() ([]byte, error) {
	if len(secretsCache) != 0 {
		return []byte(secretsCache[".OZB.json"]), nil
	}
	return []byte{}, errors.New("secrets cache is empty")
}

func (g *GoogleDrive) getClientSecretFromFile() ([]byte, error) {
	usr, err := user.Current()
	Common.PrintAndExitOnError(err, 1)
	b, err := ioutil.ReadFile(usr.HomeDir + "/.OZB.json")
	if err != nil {
		return []byte{}, err
	}
	return b, err
}

type Quota struct {
	Limit     uint64
	Used      uint64
	Unlimited bool
}

func (g *GoogleDrive) getQuota() (*Quota, error) {
	q := Quota{}
	resp, err := g.srv.About.Get().Fields("storageQuota").Do()
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

func (g *GoogleDrive) createFolder(name string) (string, error) {
	f := drive.File{Name: name, MimeType: "application/vnd.google-apps.folder"}
	res, err := g.srv.Files.Create(&f).Fields("id").Do()
	if err != nil {
		g.folderIDs[name] = res.Id
	}
	return res.Id, err
}

func (g *GoogleDrive) ListFiles(path string) {
	files, err := g.FindInFolder(path, "", "", func(file *drive.File) {
		log.Infoln(file.Properties["OZB_date"])

		size, _ := strconv.ParseUint(file.Properties["OZB_storesize"], 10, 64)
		timestamp, _ := strconv.ParseInt(file.Properties["OZB_date"], 10, 64)

		fmt.Fprintf(
			os.Stderr,
			"'%s'\n\t- Date: %s\n\t- UUID: %s\n\t- Enc.: %s\n\t- Auth: %s\n\t- Size: %s chunks, %s\n",
			file.Properties["OZB_filename"],
			time.Unix(timestamp, 0).UTC().String(),
			file.Properties["OZB_uuid"],
			strings.ToUpper(file.Properties["OZB_encryption"]),
			strings.ToUpper(file.Properties["OZB_authentication"]),
			file.Properties["OZB_chunk"],
			humanize.IBytes(size),
		)
	})
	if err != nil || files == nil {
		log.Fatal(err)
	}
}

func (g *GoogleDrive) FindOrCreateFolder(name string) string {
	if strings.HasPrefix(name, "gdrive:") {
		name = name[7:]
	}

	if id := g.folderIDs[name]; id != "" {
		return id
	}

	parent, err := g.findFileIdInParentId(name, "root")
	if err != nil {
		if err == E_NOPARENT {
			parent, err = g.createFolder(name)
			if err != nil {
				log.Fatal(err)
			}
		}
	}
	g.folderIDs[name] = parent
	return parent
}
