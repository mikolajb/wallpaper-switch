package main

import (
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/naoina/toml"
	uuid "github.com/nu7hatch/gouuid"
)

const (
	AppName         = "wallpaper-switch"
	StateFileName   = "status.toml"
	PictureFileName = "background"
	NasaRSS         = "http://apod.nasa.gov/apod.rss"
	EditorsRSS     = "https://500px.com/editors.rss"
	PopularRSS     = "https://500px.com/popular.rss"
)

type State struct {
	LastModification time.Time
	SourceURL        string
	PictureFilePath  string
	Count			 int
}

type StateFile struct {
	State    *State
	FilePath string
}

func (c *StateFile) loadState() *State {
	buf, err := ioutil.ReadFile(c.FilePath)

	c.State = new(State)
	if err != nil && !os.IsNotExist(err) {
		panic(err)
	}

	if err := toml.Unmarshal(buf, c.State); err != nil {
		fmt.Println("Problem with reading configuration file")
	}

	return c.State
}

func (c *StateFile) storeState(state *State) {
	c.State = state

	data, err := toml.Marshal(*state)

	if err != nil {
		panic(err)
	}

	if err = ioutil.WriteFile(c.FilePath, data, 0600); err != nil {
		panic(err)
	}
}

func getDirectories() (string, string) {
	config_home := os.Getenv("XDG_CONFIG_HOME")
	data_home := os.Getenv("XDG_DATA_HOME")

	if len(config_home) == 0 {
		config_home = path.Join(os.Getenv("HOME"), ".config")
	}

	if len(data_home) == 0 {
		data_home = path.Join(os.Getenv("HOME"), ".local/share")
	}

	config_home = path.Join(config_home, AppName)
	data_home = path.Join(data_home, AppName)

	if _, err := os.Stat(config_home); os.IsNotExist(err) {
		if err = os.MkdirAll(config_home, 0700); err != nil {
			panic(err)
		}
	}

	if _, err := os.Stat(data_home); os.IsNotExist(err) {
		if err = os.MkdirAll(data_home, 0700); err != nil {
			panic(err)
		}
	}

	return config_home, data_home
}

func main() {
	_, data_home := getDirectories()
	state_file := new(StateFile)
	state_file.FilePath = path.Join(data_home, StateFileName)
	state := state_file.loadState()

	type Item struct {
		Title string `xml:"title"`
		Link  string `xml:"link"`
	}

	type Channel struct {
		Items []Item `xml:"channel>item"`
	}

	var rss_str = PopularRSS
	rss_resp, err := http.Get(rss_str)

	if err != nil {
		panic(err)
	}

	body, err := ioutil.ReadAll(rss_resp.Body)

	if err != nil {
		panic(err)
	}

	rss := Channel{}
	xml.Unmarshal(body, &rss)

	if err != nil {
		panic(err)
	}

	state.Count += 1
	var next_item = state.Count % len(rss.Items)
	url, err := url.Parse(rss.Items[next_item].Link)

	if err != nil {
		panic(err)
	}

	doc, err := goquery.NewDocument(url.String())

	if err != nil {
		panic(err)
	}

	var img_url, ok = doc.Find("img.the_photo").First().Attr("src")

	if !ok {
		panic("Didn't get the file!")
	}

	if url.String() == state.SourceURL {
		fmt.Println("Same file, not changeing")
		return
	}

	picture_resp, err := http.Get(img_url)

	u4, err := uuid.NewV4()

	if err != nil {
		panic(err)
	}

	picture_path := path.Join(data_home,
		fmt.Sprintf("%s-%s%s",
			PictureFileName,
			u4.String(),
			path.Ext(url.Path)))
	picture_file, err := os.Create(picture_path)

	defer picture_file.Close()

	_, err = io.Copy(picture_file, picture_resp.Body)

	if err != nil {
		panic(err)
	}

	picture_file.Sync()

	cmds := []*exec.Cmd{
		exec.Command("gsettings", "set", "org.gnome.desktop.background",
			"picture-uri", picture_path),
		exec.Command("gsettings", "set", "org.gnome.desktop.screensaver",
			"picture-uri", picture_path),
		exec.Command("notify-send", "New Wallpaper", rss.Items[0].Link)}

	for _, cmd := range cmds {
		err = cmd.Run()

		if err != nil {
			panic(err)
		}
	}

	os.Remove(state.PictureFilePath)

	state.LastModification = time.Now()
	state.SourceURL = url.String()
	state.PictureFilePath = picture_path
	state_file.storeState(state)
}
