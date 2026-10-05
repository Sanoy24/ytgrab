package ytdlp

import (
	"encoding/xml"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// MediaServerStyle names files for Jellyfin, Kodi, Emby, and Plex: the channel is the show,
// the upload year the season, and month and day the episode.
//
//	Channel/tvshow.nfo
//	Channel/Season 2026/Channel - S2026E1004 - Title [id] 720p.mp4 (+ .nfo, -thumb.jpg)
const MediaServerStyle = "media-server"

// metaPrefix marks the line yt-dlp prints with what the .nfo files need.
const metaPrefix = "YTGRAB_META:"

// metaFields are printed after the move in the media-server style.
const metaFields = "%(.{id,title,channel,uploader,channel_id,upload_date,description,duration,webpage_url})j"

// Metadata is what yt-dlp knew about a downloaded video.
type Metadata struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Channel     string   `json:"channel"`
	Uploader    string   `json:"uploader"`
	ChannelID   string   `json:"channel_id"`
	UploadDate  string   `json:"upload_date"` // YYYYMMDD
	Description string   `json:"description"`
	Duration    *float64 `json:"duration"`
	WebpageURL  string   `json:"webpage_url"`
}

func (meta Metadata) show() string {
	if meta.Channel != "" {
		return meta.Channel
	}
	if meta.Uploader != "" {
		return meta.Uploader
	}
	return "Unknown channel"
}

type nfoID struct {
	Type    string `xml:"type,attr"`
	Default bool   `xml:"default,attr,omitempty"`
	Value   string `xml:",chardata"`
}

type episodeNFO struct {
	XMLName   xml.Name `xml:"episodedetails"`
	Title     string   `xml:"title"`
	ShowTitle string   `xml:"showtitle"`
	Season    string   `xml:"season,omitempty"`
	Episode   string   `xml:"episode,omitempty"`
	Plot      string   `xml:"plot,omitempty"`
	Aired     string   `xml:"aired,omitempty"`
	Premiered string   `xml:"premiered,omitempty"`
	Runtime   int      `xml:"runtime,omitempty"` // minutes
	UniqueID  nfoID    `xml:"uniqueid"`
	Trailer   string   `xml:"trailer,omitempty"`
}

type showNFO struct {
	XMLName  xml.Name `xml:"tvshow"`
	Title    string   `xml:"title"`
	Plot     string   `xml:"plot"`
	UniqueID *nfoID   `xml:"uniqueid,omitempty"`
}

// writeNFOs writes the episode's .nfo beside the video, and the show's tvshow.nfo in the
// channel folder unless one is there already (a user's edits are kept).
func writeNFOs(video string, meta Metadata, site string) error {
	base := strings.TrimSuffix(video, filepath.Ext(video))
	episode := episodeNFO{Title: meta.Title, ShowTitle: meta.show(), Plot: meta.Description, UniqueID: nfoID{Type: siteKey(site), Default: true, Value: meta.ID}}
	if date := meta.UploadDate; len(date) == 8 {
		day := date[:4] + "-" + date[4:6] + "-" + date[6:]
		episode.Season, episode.Episode, episode.Aired, episode.Premiered = date[:4], date[4:], day, day
	}
	if meta.Duration != nil {
		episode.Runtime = int(*meta.Duration/60 + 0.5)
	}
	if err := writeXML(base+".nfo", episode); err != nil {
		return err
	}
	show := filepath.Join(filepath.Dir(filepath.Dir(video)), "tvshow.nfo")
	if _, err := os.Stat(show); err == nil || !errors.Is(err, os.ErrNotExist) {
		return nil
	}
	nfo := showNFO{Title: meta.show(), Plot: "Videos from " + meta.show() + ", saved by YTGrab."}
	if meta.ChannelID != "" {
		nfo.UniqueID = &nfoID{Type: siteKey(site), Value: meta.ChannelID}
	}
	return writeXML(show, nfo)
}

func siteKey(site string) string {
	if site == "" {
		return "youtube"
	}
	return site
}

func writeXML(path string, value any) error {
	data, err := xml.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append([]byte(xml.Header), append(data, '\n')...), 0o644)
}
