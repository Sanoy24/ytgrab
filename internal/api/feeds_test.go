package api

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Sanoy24/ytgrab/internal/domain"
	"github.com/Sanoy24/ytgrab/internal/phone"
	sqlitestore "github.com/Sanoy24/ytgrab/internal/store/sqlite"
)

type fakeFeeds struct {
	key     string
	enabled bool
}

func (f *fakeFeeds) Status() phone.Status {
	return phone.Status{Enabled: f.enabled, URL: "http://192.168.1.5:8788"}
}
func (f *fakeFeeds) FeedKey(context.Context) (string, error) { return f.key, nil }
func (f *fakeFeeds) ValidFeedKey(_ context.Context, key string) bool {
	return key != "" && key == f.key
}
func (f *fakeFeeds) ResetFeedKey(context.Context) error { f.key = "new"; return nil }

func TestPodcastFeed(t *testing.T) {
	ctx := context.Background()
	store, err := sqlitestore.Open(ctx, filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	watch, _ := domain.NewWatch("https://www.youtube.com/@jawed", domain.AudioM4A)
	watch.Title = "jawed & friends"
	if err := store.CreateWatch(ctx, watch, []string{"jNQXAC9IVRw"}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	episode := filepath.Join(dir, "Me at the zoo [jNQXAC9IVRw].m4a")
	if err := os.WriteFile(episode, []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	mine := finishedJob(t, store, "https://youtu.be/jNQXAC9IVRw", domain.Completed, episode)
	other := filepath.Join(dir, "Other [aqz-KE-bpKQ].m4a")
	_ = os.WriteFile(other, []byte("x"), 0o600)
	finishedJob(t, store, "https://youtu.be/aqz-KE-bpKQ", domain.Completed, other) // not this watch's

	feeds := &fakeFeeds{key: "secret"}
	mux := http.NewServeMux()
	addFeedRoutes(mux, store, store, feeds)
	get := func(target string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://192.168.1.5:8788"+target, nil))
		return response
	}

	// The address: only for phones once phone access is on.
	var where map[string]string
	_ = json.Unmarshal(get("/api/watches/"+watch.ID+"/feed").Body.Bytes(), &where)
	if where["url"] != "" || !strings.Contains(where["reason"], "Settings → Phone") || !strings.Contains(where["local_url"], "key=secret") {
		t.Fatalf("feed address while off = %v", where)
	}
	feeds.enabled = true
	_ = json.Unmarshal(get("/api/watches/"+watch.ID+"/feed").Body.Bytes(), &where)
	if where["url"] != "http://192.168.1.5:8788/feeds/"+watch.ID+"?key=secret" {
		t.Fatalf("feed address = %v", where)
	}

	if got := get("/feeds/" + watch.ID + "?key=wrong"); got.Code != http.StatusNotFound {
		t.Fatalf("wrong key = %d", got.Code)
	}
	response := get("/feeds/" + watch.ID + "?key=secret")
	if response.Code != http.StatusOK || !strings.HasPrefix(response.Header().Get("Content-Type"), "application/rss+xml") {
		t.Fatalf("feed = %d %s", response.Code, response.Header())
	}
	var feed rss
	if err := xml.Unmarshal(response.Body.Bytes(), &feed); err != nil {
		t.Fatalf("feed isn't XML: %v\n%s", err, response.Body.String())
	}
	if feed.Channel.Title != "jawed & friends" || len(feed.Channel.Items) != 1 {
		t.Fatalf("channel = %+v", feed.Channel)
	}
	item := feed.Channel.Items[0]
	if item.Enclosure.Type != "audio/mp4" || item.Enclosure.Length != 10 || !strings.HasSuffix(item.Enclosure.URL, "/"+mine.ID+".m4a?key=secret") {
		t.Fatalf("enclosure = %+v", item.Enclosure)
	}

	// The episode streams with range requests; other watches' files and wrong keys don't.
	link, _ := url.Parse(item.Enclosure.URL)
	request := httptest.NewRequest(http.MethodGet, link.RequestURI(), nil)
	request.Header.Set("Range", "bytes=0-3")
	got := httptest.NewRecorder()
	mux.ServeHTTP(got, request)
	if got.Code != http.StatusPartialContent || got.Body.String() != "0123" || got.Header().Get("Content-Type") != "audio/mp4" {
		t.Fatalf("episode = %d %q %s", got.Code, got.Body.String(), got.Header())
	}
	if got := get(strings.Replace(link.RequestURI(), "key=secret", "key=nope", 1)); got.Code != http.StatusNotFound {
		t.Errorf("episode with a wrong key = %d", got.Code)
	}

	// Resetting the key retires every earlier address.
	reset := httptest.NewRecorder()
	mux.ServeHTTP(reset, httptest.NewRequest(http.MethodPost, "/api/feeds/reset", nil))
	if got := get("/feeds/" + watch.ID + "?key=secret"); reset.Code != http.StatusNoContent || got.Code != http.StatusNotFound {
		t.Errorf("after reset = %d, %d", reset.Code, got.Code)
	}
}
