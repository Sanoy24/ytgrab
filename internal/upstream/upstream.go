// Package upstream notices when downloaded videos disappear from YouTube or Vimeo: deleted
// by their owner or removed by the site. Then the Library's copy may be the only one left.
// It is off until the user turns it on, checks each video about once a week, a few at a
// time, and uses the sites' public oEmbed lookups, which yt-dlp's limits don't count.
package upstream

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/Sanoy24/ytgrab/internal/domain"
	sqlitestore "github.com/Sanoy24/ytgrab/internal/store/sqlite"
)

// Store remembers what was checked (the SQLite store).
type Store interface {
	UpstreamDue(ctx context.Context, before time.Time, limit int) ([]sqlitestore.UpstreamVideo, error)
	SetUpstream(ctx context.Context, site, videoID string, gone bool, at time.Time) (bool, error)
}

const (
	every    = 7 * 24 * time.Hour // how often each video is checked
	perRound = 20
	round    = 10 * time.Minute
	pause    = 2 * time.Second // between two lookups
)

// errSlowDown stops a round when the site asks for fewer requests.
var errSlowDown = errors.New("the site asked to slow down")

// Checker checks downloaded videos while on() reports true and calls gone with the titles
// of videos found gone in a round.
type Checker struct {
	store Store
	on    func() bool
	gone  func([]string)

	// Replaced in tests.
	client *http.Client
	now    func() time.Time
	sleep  func(context.Context, time.Duration) bool
	lookup func(site, videoID string) string
}

func New(store Store, on func() bool, gone func([]string)) *Checker {
	return &Checker{
		store: store, on: on, gone: gone,
		client: &http.Client{Timeout: 15 * time.Second},
		now:    time.Now,
		sleep: func(ctx context.Context, d time.Duration) bool {
			select {
			case <-ctx.Done():
				return false
			case <-time.After(d):
				return true
			}
		},
		lookup: oembedURL,
	}
}

// Run checks a round every ten minutes, starting a few minutes after YTGrab does.
func (c *Checker) Run(ctx context.Context) {
	if !c.sleep(ctx, 3*time.Minute) {
		return
	}
	for {
		if c.on() {
			c.Round(ctx)
		}
		if !c.sleep(ctx, round) {
			return
		}
	}
}

// Round checks the videos most overdue and reports those found gone.
func (c *Checker) Round(ctx context.Context) {
	due, err := c.store.UpstreamDue(ctx, c.now().Add(-every), perRound)
	if err != nil {
		return
	}
	var gone []string
	for i, video := range due {
		if i > 0 && !c.sleep(ctx, pause) {
			break
		}
		online, err := c.check(ctx, video.Site, video.VideoID)
		if errors.Is(err, errSlowDown) {
			break
		}
		if err != nil {
			continue // not a clear answer: tried again next round
		}
		justGone, err := c.store.SetUpstream(ctx, video.Site, video.VideoID, !online, c.now())
		if err == nil && justGone {
			title := video.Title
			if title == "" {
				title = video.VideoID
			}
			gone = append(gone, title)
		}
	}
	if len(gone) > 0 && c.gone != nil {
		c.gone(gone)
	}
}

// check asks the site's oEmbed lookup about a video: 200 is online, 404 is gone. Anything
// else, such as 401 (private, or embedding turned off), says nothing either way.
func (c *Checker) check(ctx context.Context, site, videoID string) (bool, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.lookup(site, videoID), nil)
	if err != nil {
		return false, err
	}
	request.Header.Set("User-Agent", "ytgrab")
	response, err := c.client.Do(request)
	if err != nil {
		return false, err
	}
	response.Body.Close()
	switch response.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	case http.StatusTooManyRequests, http.StatusForbidden:
		return false, errSlowDown
	default:
		return false, errors.New(response.Status)
	}
}

func oembedURL(site, videoID string) string {
	if site == domain.SiteVimeo {
		return "https://vimeo.com/api/oembed.json?url=" + url.QueryEscape("https://vimeo.com/"+videoID)
	}
	return "https://www.youtube.com/oembed?format=json&url=" + url.QueryEscape("https://www.youtube.com/watch?v="+videoID)
}
