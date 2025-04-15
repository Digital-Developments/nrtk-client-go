package main

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/spf13/viper"
)

type LocalWriter interface {
	GetFilePath() string
	GetContent() []byte
}

func SaveFile(any interface{}) error {

	if obj, ok := any.(LocalWriter); ok {
		f, err := os.OpenFile(obj.GetFilePath(), os.O_CREATE|os.O_WRONLY, 0666)
		if err != nil {
			return fmt.Errorf("unable to open %v for writing", obj.GetFilePath())
		}

		log.Printf("Saving %v\n", obj.GetFilePath())
		_, e := f.Write(obj.GetContent())
		if e != nil {
			return e
		}
		defer f.Close()
		return nil
	}

	return fmt.Errorf("LocalWriter interface is not supported")

}

type Story struct {
	Uid          string `json:"uid"`
	Anchor       string `json:"anchor"`
	CanonicalUrl string `json:"canonical_url"`
	Title        string `json:"title"`
	Credits      string `json:"credits"`
	Content      string `json:"content"`
	StoryDate    string `json:"story_date"`
	IsLanding    bool   `json:"is_landing"`
	UpdatedAt    string `json:"updated_at"`
	Url          string `json:"url"`
	Hash         string `json:"hash"`
}

func (s *Story) GetSitemapItem() string {
	priority := .8
	if s.Anchor == "index" {
		priority = 1
	}
	return fmt.Sprintf("<url><loc>%v</loc><lastmod>%v+00:00</lastmod><priority>%v</priority></url>", s.CanonicalUrl, s.UpdatedAt[:19], priority)
}

func (s Story) GetFilePath() string {
	return fmt.Sprintf("%v%v%v", viper.GetString("ContentDir"), s.Anchor, viper.GetString("ContentFileExtension"))
}

func (s Story) GetContent() []byte {
	return []byte(s.Content)
}

type ContentFile struct {
	FileName string
	Content  string
}

func (f ContentFile) GetFilePath() string {
	return fmt.Sprintf("%v%v", viper.GetString("ContentDir"), f.FileName)
}

func (f ContentFile) GetContent() []byte {
	return []byte(f.Content)
}

type SiteData struct {
	Title       string  `json:"title"`
	Entity      string  `json:"entity"`
	Locale      string  `json:"locale"`
	SiteName    string  `json:"site_name"`
	LogoUrl     string  `json:"logo_url"`
	HomepageUrl string  `json:"homepage_url"`
	Stories     []Story `json:"stories"`
	ErrorPage   string  `json:"error_page"`
}

func (sd *SiteData) GetSitemap() string {

	sitemap := "<?xml version=\"1.0\" encoding=\"UTF-8\"?><urlset xmlns=\"http://www.sitemaps.org/schemas/sitemap/0.9\" xmlns:xsi=\"http://www.w3.org/2001/XMLSchema-instance\" xsi:schemaLocation=\"http://www.sitemaps.org/schemas/sitemap/0.9 http://www.sitemaps.org/schemas/sitemap/0.9/sitemap.xsd\"><!-- Created by Newsroom Toolkit www.newsroomtoolkit.com -->"

	for _, story := range sd.Stories {
		sitemap += story.GetSitemapItem()
	}

	return sitemap + "</urlset>"

}

type MetaObject struct {
	Title       string    `json:"title"`
	Entity      string    `json:"entity"`
	HomepageUrl string    `json:"homepage_url"`
	Stories     []Story   `json:"stories"`
	Checksum    string    `json:"checksum"`
	UpdatedAt   time.Time `json:"updated_at"`
	IsExpired   bool      `json:"-"`
}

func (m *MetaObject) IsUpdateNeeded() (bool, error) {

	local_json, err := read_json_file(viper.GetString("MetaPath"))

	if err != nil {
		return true, err
	} else {
		current_meta := MetaObject{}
		jsonErr := json.Unmarshal(local_json, &current_meta)
		if jsonErr != nil {
			return true, jsonErr
		} else {
			current_meta.IsExpired = current_meta.Checksum != m.Checksum
			if current_meta.IsExpired {
				SaveFile(current_meta)
			}
			return current_meta.IsExpired, nil
		}
	}

}

func (m *MetaObject) SetChecksum(data []byte) {
	h := sha256.New()
	h.Write(data)
	m.Checksum = fmt.Sprintf("%x", h.Sum(nil))
	m.UpdatedAt = time.Now()
}

func (m MetaObject) GetFilePath() string {
	if m.IsExpired {
		return fmt.Sprintf("%vmeta.%v.json", viper.GetString("SnapshotDir"), m.Checksum)
	}
	return viper.GetString("MetaPath")
}

func (m MetaObject) GetContent() []byte {
	json_dump, _ := json.Marshal(m)
	return json_dump
}

func read_json_file(filePath string) ([]byte, error) {
	jsonFile, open_err := os.Open(filePath)
	if open_err != nil {
		return nil, open_err
	}

	log.Printf("Reading data from %v\n", filePath)

	defer jsonFile.Close()

	byteValue, err := io.ReadAll(jsonFile)

	if err != nil {
		return byteValue, errors.New("unable to read file data")
	}

	return byteValue, nil
}

func fetch_remote(url, token string) ([]byte, error) {

	apiClient := http.Client{
		Timeout: time.Second * 2, // Timeout after 2 seconds
	}

	log.Printf("Fetching data from %v\n", url)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, errors.New("HTTP Error")
	}

	req.Header.Set("User-Agent", "NRTK SyncGo v0.1")
	req.Header.Set("Authorization", fmt.Sprintf("Token %v", token))

	response, getErr := apiClient.Do(req)
	if getErr != nil || response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request Error: %v", response.StatusCode)
	}

	if response.Body != nil {
		defer response.Body.Close()
	}

	body, readErr := io.ReadAll(response.Body)
	if readErr != nil {
		return nil, readErr
	}

	return body, nil
}

func create_dirs() error {

	content_dir_err := os.MkdirAll(viper.GetString("ContentDir"), 0755)
	if content_dir_err != nil {
		return content_dir_err
	}

	bin_dir_err := os.MkdirAll(viper.GetString("SnapshotDir"), 0755)
	if bin_dir_err != nil {
		return bin_dir_err
	}

	return nil

}

func empty_content_dir() error {

	log.Printf("Purging content at %v", viper.GetString("ContentDir"))

	files, err := os.ReadDir(viper.GetString("ContentDir"))

	if err != nil {
		return err
	}

	for _, f := range files {

		e := os.Remove(viper.GetString("ContentDir") + "/" + f.Name())
		if e != nil {
			return e
		}

		log.Printf("Removing %v\n", f.Name())
	}

	return nil
}

func parse(api_response []byte) {

	sync_data := SiteData{}
	jsonErr := json.Unmarshal(api_response, &sync_data)

	if jsonErr != nil {
		log.Fatal(jsonErr)
		panic("unable to parse JSON data")
	}

	err := create_dirs()
	if err != nil {
		log.Fatal(err)
		panic("unable to create app dirs")
	}

	meta_object := MetaObject{
		Title:       sync_data.Title,
		Entity:      sync_data.Entity,
		HomepageUrl: sync_data.HomepageUrl,
		Stories:     sync_data.Stories,
	}

	meta_object.SetChecksum(api_response)

	result, _ := meta_object.IsUpdateNeeded()

	if result || viper.GetBool("MODE_FORCE_UPDATE") {

		log.Printf("Sync content for %v with %v stories (MODE_FORCE_UPDATE=%v)\n", sync_data.SiteName, len(sync_data.Stories), viper.GetBool("MODE_FORCE_UPDATE"))

		empty_content_dir()

		SaveFile(meta_object)

		for _, story := range sync_data.Stories {
			e := SaveFile(story)
			if e != nil {
				fmt.Println(e)
			}
		}

		error_page := ContentFile{
			FileName: "404" + viper.GetString("ContentFileExtension"),
			Content:  sync_data.ErrorPage,
		}
		SaveFile(error_page)

		sitemap := ContentFile{
			FileName: "sitemap.xml",
			Content:  sync_data.GetSitemap(),
		}
		SaveFile(sitemap)

	} else {
		log.Printf("Nothing to update")
	}

}

func handleSyncRequest(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("token") == viper.GetString("NRTK_API_TOKEN") {
		log.Printf("Sync signal recieved from %v", r.RemoteAddr)
		w.WriteHeader(200)
		fmt.Fprint(w, "👋 Sync signal recieved")
		sync()
	} else {
		log.Printf("Invalid sync token [%v] recieved from %v", r.URL.Query().Get("token"), r.RemoteAddr)
		w.WriteHeader(401)
		fmt.Fprint(w, "💔 Unable to handle your request")
	}
}

func handleFileRequest(w http.ResponseWriter, r *http.Request) {

	request_path := r.URL.Path[1:]
	file_path := viper.GetString("ContentDir") + path.Clean(request_path)

	_, err := os.OpenFile(file_path, os.O_RDONLY, 0644)
	if os.IsNotExist(err) {
		log.Printf("Error 404: %v", err)
		if len(viper.GetString("ContentFileExtension")) > 0 {

			file_path_extension := file_path + viper.GetString("ContentFileExtension")
			_, err_extension := os.OpenFile(file_path_extension, os.O_RDONLY, 0644)

			if os.IsNotExist(err_extension) {
				log.Printf("Fallback Error 404: %v", err_extension)
				http.ServeFile(w, r, viper.GetString("ContentDir")+"/404"+viper.GetString("ContentFileExtension"))
			} else {
				log.Printf("Serving %v on behalf of %v", file_path_extension, r.URL.Path)
				http.ServeFile(w, r, file_path_extension)
			}

		} else {
			http.ServeFile(w, r, viper.GetString("ContentDir")+"/404")
		}

	} else {

		http.ServeFile(w, r, file_path)
	}
}

func requestHandler(w http.ResponseWriter, r *http.Request) {

	log.Printf("Handling %v from %v", r.URL.Path, r.RemoteAddr)

	if r.URL.Path != "/" {

		if r.URL.Path == viper.GetString("HTTP_SERVER_SYNC_HANDLER") {
			handleSyncRequest(w, r)
		} else {
			handleFileRequest(w, r)
		}

	} else {
		http.ServeFile(w, r, viper.GetString("ContentDir")+"/index"+viper.GetString("ContentFileExtension"))
	}

}

func start_server() {

	log.Printf("Starting web server on :" + viper.GetString("HTTP_SERVER_PORT"))

	http.HandleFunc("/", requestHandler)

	if err := http.ListenAndServe(":"+viper.GetString("HTTP_SERVER_PORT"), nil); err != nil {
		log.Fatal(err)
	}
}

func sync() {
	var api_response []byte
	var fetchError error

	if !viper.GetBool("MODE_FETCH_LOCAL") && len(viper.GetString("NRTK_API_TOKEN")) > 0 {
		api_response, fetchError = fetch_remote(viper.GetString("NRTK_API_URL"), viper.GetString("NRTK_API_TOKEN"))
	} else {
		api_response, fetchError = read_json_file("local.json")
	}

	if fetchError != nil {
		panic(fetchError)
	} else {
		parse(api_response)
	}
}

func main() {

	viper.SetDefault("APP_NAME", ".nrtk")
	viper.SetDefault("ContentFileExtension", "")

	log.SetPrefix("nrtk-sync: ")
	log.SetFlags(0)

	viper.SetConfigName(".env")
	viper.SetConfigType("env")
	viper.AddConfigPath(".")

	err := viper.ReadInConfig()
	if err != nil {
		panic(fmt.Errorf("fatal error config file: %w", err))
	}

	viper.Set("ContentDir", viper.GetString("APP_NAME")+"/www/")
	viper.Set("SnapshotDir", viper.GetString("APP_NAME")+"/snapshot/")
	viper.Set("MetaPath", viper.GetString("APP_NAME")+"/meta.json")

	if viper.GetBool("ADD_HTML_EXTENSION") {
		viper.Set("ContentFileExtension", ".html")
	}

	if viper.GetBool("HTTP_SERVER_ENABLED") && viper.GetInt("HTTP_SERVER_PORT") > 0 {
		sync()
		start_server()

	} else {

		if viper.GetInt("MODE_INFINITY") > 0 {
			for {
				sleep_timer := time.Duration(viper.GetInt("MODE_INFINITY")) * time.Millisecond
				sync()
				log.Printf("Taking a %v second nap", sleep_timer.Seconds())
				time.Sleep(sleep_timer)
			}

		} else {
			sync()
		}
	}
}
