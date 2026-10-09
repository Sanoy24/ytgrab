package api

import (
	"context"
	"encoding/xml"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Sanoy24/ytgrab/internal/domain"
	"github.com/Sanoy24/ytgrab/internal/phone"
)

// FeedAccess holds the key that podcast feed addresses carry (phone.Service).
type FeedAccess interface {
	Status() phone.Status
	FeedKey(context.Context) (string, error)
	ValidFeedKey(context.Context, string) bool
	ResetFeedKey(context.Context) error
}

// feedStore finds a watch and the videos it has seen.
type feedStore interface {
	GetWatch(context.Context, string) (domain.Watch, error)
	SeenIDs(context.Context, string) ([]string, error)
}

// feedItems is how many of a watch's newest downloads a feed lists.
const feedItems = 100

// Podcast apps pick players by type; these are what YTGrab saves.
var feedTypes = map[string]string{
	".m4a": "audio/mp4", ".mp3": "audio/mpeg", ".opus": "audio/ogg", ".ogg": "audio/ogg",
	".flac": "audio/flac", ".wav": "audio/wav", ".mp4": "video/mp4", ".webm": "video/webm",
	".mkv": "video/x-matroska",
}

func addFeedRoutes(mux *http.ServeMux, jobs JobStore, store feedStore, feeds FeedAccess) {
	// Where a watch's feed is: { url (for phones, when phone access is on), local_url, reason }.
	mux.HandleFunc("GET /api/watches/{id}/feed", func(w http.ResponseWriter, r *http.Request) {
		watch, err := store.GetWatch(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusNotFound, "not_found", "That watch no longer exists.")
			return
		}
		key, err := feeds.FeedKey(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "storage", "Could not make the feed address.")
			return
		}
		path := "/feeds/" + url.PathEscape(watch.ID) + "?key=" + url.QueryEscape(key)
		body := map[string]string{"local_url": "http://" + r.Host + path}
		status := feeds.Status()
		switch {
		case !status.Enabled:
			body["reason"] = "Turn on phone access in Settings → Phone, so a podcast app on your phone can reach this computer."
		case status.URL == "":
			body["reason"] = status.Error
		default:
			body["url"] = status.URL + path
		}
		writeJSON(w, http.StatusOK, body)
	})
	mux.HandleFunc("POST /api/feeds/reset", func(w http.ResponseWriter, r *http.Request) {
		if err := feeds.ResetFeedKey(r.Context()); err != nil {
			writeError(w, http.StatusInternalServerError, "storage", "Could not reset the feed addresses.")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// A watch's finished downloads as a podcast feed.
	mux.HandleFunc("GET /feeds/{watch}", func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("key")
		watch, items, ok := feedFor(w, r, jobs, store, feeds, key)
		if !ok {
			return
		}
		base := "http://" + r.Host + "/feeds/" + url.PathEscape(watch.ID) + "/"
		channel := rssChannel{
			Title:       watchTitle(watch),
			Link:        watch.URL,
			Description: "Videos from " + watchTitle(watch) + ", downloaded by YTGrab on your computer.",
			Author:      watchTitle(watch),
		}
		for _, item := range items {
			ext := strings.ToLower(filepath.Ext(item.path))
			entry := rssItem{
				Title:   jobTitle(item.job),
				GUID:    rssGUID{PermaLink: "false", Value: item.job.Site + ":" + item.job.VideoID},
				PubDate: item.job.UpdatedAt.UTC().Format(time.RFC1123Z),
				Link:    item.job.URL,
				Enclosure: rssEnclosure{
					URL:    base + url.PathEscape(item.job.ID+ext) + "?key=" + url.QueryEscape(key),
					Length: item.size,
					Type:   feedTypes[ext],
				},
			}
			if item.job.Site == domain.SiteYouTube {
				entry.Image = &itunesImage{Href: "https://i.ytimg.com/vi/" + url.PathEscape(item.job.VideoID) + "/hqdefault.jpg"}
			}
			channel.Items = append(channel.Items, entry)
		}
		data, err := xml.MarshalIndent(rss{Version: "2.0", ITunes: "http://www.itunes.com/dtds/podcast-1.0.dtd", Channel: channel}, "", "  ")
		if err != nil {
			writeError(w, http.StatusInternalServerError, "feed_failed", "Could not make the feed.")
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(append([]byte(xml.Header), data...))
	})

	// An episode: one of the watch's files, with range requests so apps can stream it.
	mux.HandleFunc("GET /feeds/{watch}/{file}", func(w http.ResponseWriter, r *http.Request) {
		_, items, ok := feedFor(w, r, jobs, store, feeds, r.URL.Query().Get("key"))
		if !ok {
			return
		}
		id := strings.TrimSuffix(r.PathValue("file"), filepath.Ext(r.PathValue("file")))
		for _, item := range items {
			if item.job.ID != id {
				continue
			}
			file, err := os.Open(item.path)
			if err != nil {
				break
			}
			defer file.Close()
			info, err := file.Stat()
			if err != nil {
				break
			}
			if kind := feedTypes[strings.ToLower(filepath.Ext(item.path))]; kind != "" {
				w.Header().Set("Content-Type", kind)
			}
			w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": filepath.Base(item.path)}))
			http.ServeContent(w, r, filepath.Base(item.path), info.ModTime(), file)
			return
		}
		http.NotFound(w, r)
	})
}

type feedItem struct {
	job  domain.Job
	path string
	size int64
}

// feedFor checks the key and lists the watch's finished downloads whose files are still
// there, newest first. A wrong key looks like a missing feed.
func feedFor(w http.ResponseWriter, r *http.Request, jobs JobStore, store feedStore, feeds FeedAccess, key string) (domain.Watch, []feedItem, bool) {
	if !feeds.ValidFeedKey(r.Context(), key) {
		http.NotFound(w, r)
		return domain.Watch{}, nil, false
	}
	watch, err := store.GetWatch(r.Context(), r.PathValue("watch"))
	if err != nil {
		http.NotFound(w, r)
		return domain.Watch{}, nil, false
	}
	seen, err := store.SeenIDs(r.Context(), watch.ID)
	if err != nil {
		http.Error(w, "could not read the watch", http.StatusInternalServerError)
		return domain.Watch{}, nil, false
	}
	ids := make(map[string]bool, len(seen))
	for _, id := range seen {
		ids[id] = true
	}
	list, err := jobs.List(r.Context(), 500)
	if err != nil {
		http.Error(w, "could not read the library", http.StatusInternalServerError)
		return domain.Watch{}, nil, false
	}
	var items []feedItem
	for _, job := range list {
		if job.Site != watch.Site || !ids[job.VideoID] || job.Section != nil {
			continue
		}
		path, owned := ownedOutput(job)
		if !owned {
			continue
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		items = append(items, feedItem{job: job, path: path, size: info.Size()})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].job.UpdatedAt.After(items[j].job.UpdatedAt) })
	if len(items) > feedItems {
		items = items[:feedItems]
	}
	return watch, items, true
}

func watchTitle(watch domain.Watch) string {
	if watch.Title != "" {
		return watch.Title
	}
	return watch.URL
}

func jobTitle(job domain.Job) string {
	if job.Title != nil && *job.Title != "" {
		return *job.Title
	}
	return job.VideoID
}

type rss struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	ITunes  string     `xml:"xmlns:itunes,attr"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title       string    `xml:"title"`
	Link        string    `xml:"link"`
	Description string    `xml:"description"`
	Author      string    `xml:"itunes:author"`
	Items       []rssItem `xml:"item"`
}

type rssItem struct {
	Title     string       `xml:"title"`
	GUID      rssGUID      `xml:"guid"`
	PubDate   string       `xml:"pubDate"`
	Link      string       `xml:"link"`
	Enclosure rssEnclosure `xml:"enclosure"`
	Image     *itunesImage `xml:"itunes:image,omitempty"`
}

type rssGUID struct {
	PermaLink string `xml:"isPermaLink,attr"`
	Value     string `xml:",chardata"`
}

type rssEnclosure struct {
	URL    string `xml:"url,attr"`
	Length int64  `xml:"length,attr"`
	Type   string `xml:"type,attr"`
}

type itunesImage struct {
	Href string `xml:"href,attr"`
}
