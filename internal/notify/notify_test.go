package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Sanoy24/ytgrab/internal/activity"
)

func TestValid(t *testing.T) {
	for _, c := range []struct {
		settings Settings
		ok       bool
	}{
		{Settings{}, true}, // off
		{Settings{Ntfy, "https://ntfy.sh/my-ytgrab", "all"}, true},
		{Settings{Ntfy, "https://ntfy.sh/", "all"}, false}, // no topic
		{Settings{Discord, "https://discord.com/api/webhooks/1/abc", "failed"}, true},
		{Settings{Discord, "https://evil.example/api/webhooks/1/abc", "failed"}, false},
		{Settings{Webhook, "http://192.168.1.10:8080/hook", "all"}, true},
		{Settings{Webhook, "ftp://example.com/x", "all"}, false},
		{Settings{Webhook, "https://example.com/x", "sometimes"}, false},
		{Settings{"pigeon", "https://example.com/x", "all"}, false},
	} {
		if err := c.settings.Valid(); (err == nil) != c.ok {
			t.Errorf("%+v valid = %v, want %v", c.settings, err, c.ok)
		}
	}
}

func TestMessagesAreBatched(t *testing.T) {
	summary := activity.Summary{
		Finished: []string{"A", "B", "C", "D", "E", "F", "G"},
		Failed:   []activity.Failure{{Title: "Talk", Message: "This video is unavailable or private."}},
	}
	all := Messages(summary, "all")
	if len(all) != 2 || all[0].Title != "7 downloads finished" || !strings.HasSuffix(all[0].Body, "and 2 more") {
		t.Fatalf("all = %+v", all)
	}
	if all[1].Title != "Download failed" || all[1].Body != "Talk: This video is unavailable or private." {
		t.Errorf("failed = %+v", all[1])
	}
	if failedOnly := Messages(summary, "failed"); len(failedOnly) != 1 || failedOnly[0].Event != "failed" {
		t.Errorf("failures only = %+v", failedOnly)
	}
	if none := Messages(activity.Summary{Downloading: 1}, "all"); len(none) != 0 {
		t.Errorf("nothing ended = %+v", none)
	}
}

func TestSendSpeaksEachServicesFormat(t *testing.T) {
	type received struct {
		header http.Header
		body   string
	}
	var got received
	status := http.StatusOK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		got = received{r.Header.Clone(), string(data)}
		w.WriteHeader(status)
	}))
	defer server.Close()
	ctx := context.Background()
	message := Message{Event: "failed", Title: "Download failed", Body: "Talk: unavailable"}

	if err := Send(ctx, Settings{Ntfy, server.URL + "/topic", "all"}, message); err != nil {
		t.Fatal(err)
	}
	if got.body != "Talk: unavailable" || got.header.Get("Title") != "YTGrab: Download failed" || got.header.Get("Tags") != "warning" {
		t.Errorf("ntfy got %+v", got)
	}
	// Titles beyond ASCII reach ntfy encoded.
	_ = Send(ctx, Settings{Ntfy, server.URL + "/topic", "all"}, Message{Event: "finished", Title: "ሰላም", Body: "x"})
	if title := got.header.Get("Title"); !strings.HasPrefix(title, "=?UTF-8?b?") {
		t.Errorf("non-ASCII title = %q", title)
	}

	var webhook map[string]string
	if err := Send(ctx, Settings{Webhook, server.URL + "/hook", "all"}, message); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(got.body), &webhook); err != nil || webhook["event"] != "failed" || webhook["app"] != "YTGrab" {
		t.Errorf("webhook got %s", got.body)
	}

	status = http.StatusNotFound
	if err := Send(ctx, Settings{Webhook, server.URL + "/hook", "all"}, message); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("a refused notification = %v", err)
	}
}

type capture struct{ request *http.Request }

func (c *capture) RoundTrip(r *http.Request) (*http.Response, error) {
	c.request = r
	return &http.Response{StatusCode: http.StatusNoContent, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
}

func TestDiscordCantBeMadeToMention(t *testing.T) {
	sent := &capture{}
	old := client
	client = &http.Client{Transport: sent}
	t.Cleanup(func() { client = old })
	err := Send(context.Background(), Settings{Discord, "https://discord.com/api/webhooks/1/abc", "all"}, Message{Event: "finished", Title: "Download finished", Body: "@everyone free stuff"})
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Content  string `json:"content"`
		Username string `json:"username"`
		Allowed  struct {
			Parse []string `json:"parse"`
		} `json:"allowed_mentions"`
	}
	data, _ := io.ReadAll(sent.request.Body)
	if err := json.Unmarshal(data, &body); err != nil || body.Username != "YTGrab" || !strings.HasPrefix(body.Content, "**Download finished**") || body.Allowed.Parse == nil || len(body.Allowed.Parse) != 0 {
		t.Errorf("discord got %s", data)
	}
}
