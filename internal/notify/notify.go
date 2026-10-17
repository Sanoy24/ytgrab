// Package notify sends finished and failed downloads to the user's phone or chat, through
// a service they choose: ntfy, a Discord webhook, or any webhook that takes JSON. It is off
// until the user sets it up, and it sends only titles and error messages.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Sanoy24/ytgrab/internal/activity"
)

// Kinds of services.
const (
	Ntfy    = "ntfy"
	Discord = "discord"
	Webhook = "webhook"
)

// Settings say where notifications go; Kind "" is off. Events is "failed" (failures only)
// or "all" (finished and failed).
type Settings struct {
	Kind   string `json:"kind"`
	URL    string `json:"url"`
	Events string `json:"events"`
}

// Valid checks the settings: a known kind, an http(s) address, and for Discord a webhook.
func (s Settings) Valid() error {
	if s.Kind == "" {
		return nil
	}
	if s.Kind != Ntfy && s.Kind != Discord && s.Kind != Webhook {
		return errors.New("Choose ntfy, Discord, or a webhook.")
	}
	if s.Events != "failed" && s.Events != "all" {
		return errors.New("Choose which downloads to be told about.")
	}
	address, err := url.Parse(s.URL)
	if err != nil || (address.Scheme != "https" && address.Scheme != "http") || address.Host == "" || len(s.URL) > 500 {
		return errors.New("Enter the full address, starting with https://")
	}
	if s.Kind == Discord {
		host := strings.ToLower(address.Hostname())
		if (host != "discord.com" && host != "discordapp.com" && !strings.HasSuffix(host, ".discord.com")) || !strings.HasPrefix(address.Path, "/api/webhooks/") {
			return errors.New("A Discord webhook address looks like https://discord.com/api/webhooks/…")
		}
	}
	if s.Kind == Ntfy && strings.Trim(address.Path, "/") == "" {
		return errors.New("Add your topic to the ntfy address, like https://ntfy.sh/your-topic")
	}
	return nil
}

// Message is one notification.
type Message struct {
	Event string // "finished", "failed", or "test"
	Title string
	Body  string
}

// Messages turns what changed into notifications: one for finished downloads and one for
// failed ones, so a playlist finishing doesn't send dozens.
func Messages(summary activity.Summary, events string) []Message {
	var messages []Message
	if events == "all" && len(summary.Finished) > 0 {
		title := "Download finished"
		if n := len(summary.Finished); n > 1 {
			title = fmt.Sprintf("%d downloads finished", n)
		}
		messages = append(messages, Message{Event: "finished", Title: title, Body: list(summary.Finished)})
	}
	if len(summary.Failed) > 0 {
		title := "Download failed"
		if n := len(summary.Failed); n > 1 {
			title = fmt.Sprintf("%d downloads failed", n)
		}
		var lines []string
		for _, failure := range summary.Failed {
			line := failure.Title
			if failure.Message != "" {
				line += ": " + failure.Message
			}
			lines = append(lines, line)
		}
		messages = append(messages, Message{Event: "failed", Title: title, Body: list(lines)})
	}
	return messages
}

// list keeps a message short: the first five, then how many more.
func list(items []string) string {
	if len(items) <= 5 {
		return strings.Join(items, "\n")
	}
	return strings.Join(items[:5], "\n") + fmt.Sprintf("\nand %d more", len(items)-5)
}

// client is replaced in tests.
var client = &http.Client{Timeout: 15 * time.Second}

// Send delivers a message the way the settings' service expects it.
func Send(ctx context.Context, settings Settings, message Message) error {
	if err := settings.Valid(); err != nil || settings.Kind == "" {
		return err
	}
	var body []byte
	contentType := "application/json"
	switch settings.Kind {
	case Ntfy:
		body, contentType = []byte(message.Body), "text/plain; charset=utf-8"
	case Discord:
		body, _ = json.Marshal(map[string]any{
			"username": "YTGrab", "content": "**" + message.Title + "**\n" + message.Body,
			"allowed_mentions": map[string]any{"parse": []string{}}, // titles can't ping anyone
		})
	default:
		body, _ = json.Marshal(map[string]string{"app": "YTGrab", "event": message.Event, "title": message.Title, "message": message.Body})
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, settings.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("User-Agent", "ytgrab")
	if settings.Kind == Ntfy {
		request.Header.Set("Title", headerSafe("YTGrab: "+message.Title))
		tag := "white_check_mark"
		if message.Event == "failed" {
			tag = "warning"
		}
		request.Header.Set("Tags", tag)
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("couldn't reach %s: %w", request.URL.Host, err)
	}
	response.Body.Close()
	if response.StatusCode >= 300 {
		return fmt.Errorf("%s answered %s", request.URL.Host, response.Status)
	}
	return nil
}

// headerSafe keeps a header to one line of printable text; text beyond ASCII (accents,
// Amharic) is encoded the way ntfy reads it (RFC 2047).
func headerSafe(text string) string {
	text = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, text)
	for _, r := range text {
		if r > 0x7e {
			return mime.BEncoding.Encode("UTF-8", text)
		}
	}
	return text
}
