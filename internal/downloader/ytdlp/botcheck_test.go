package ytdlp

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Sanoy24/ytgrab/internal/config"
	"github.com/Sanoy24/ytgrab/internal/domain"
)

// network fakes YouTube: which player clients get through, and the calls made.
type network struct {
	flagged    bool // the bot check is on for this address
	embeddable bool
	calls      []string
}

func (n *network) attempt(clients string) (string, error) {
	n.calls = append(n.calls, clients)
	switch {
	case clients == "" && n.flagged:
		return "", classifyFailure("ERROR: [youtube] x: Sign in to confirm you're not a bot.")
	case clients != "" && !n.embeddable:
		return "", classifyFailure("ERROR: [youtube] x: Video unavailable. Playback on other websites has been disabled by the video owner")
	}
	return "video via " + clients, nil
}

func resetBotCheck(t *testing.T) *time.Time {
	t.Helper()
	now := time.Now()
	clearBotCheck()
	botCheckNow = func() time.Time { return now }
	t.Cleanup(func() { clearBotCheck(); botCheckNow = time.Now })
	return &now
}

func TestBotCheckSwitchesToEmbeddedPlayers(t *testing.T) {
	now := resetBotCheck(t)
	ctx := context.Background()
	n := &network{embeddable: true}

	// Usual case: yt-dlp's own clients, one call.
	if got, err := withBotCheckFallback(ctx, n.attempt); err != nil || got != "video via " {
		t.Fatalf("unflagged = %q, %v", got, err)
	}

	// The bot check appears: retried at once with the embedded players, then remembered.
	n.flagged, n.calls = true, nil
	if got, err := withBotCheckFallback(ctx, n.attempt); err != nil || got != "video via "+EmbeddedClients {
		t.Fatalf("flagged = %q, %v", got, err)
	}
	if !BotCheckActive() || len(n.calls) != 2 {
		t.Fatalf("calls = %q, active = %v", n.calls, BotCheckActive())
	}
	n.calls = nil
	if _, err := withBotCheckFallback(ctx, n.attempt); err != nil || len(n.calls) != 1 || n.calls[0] != EmbeddedClients {
		t.Fatalf("while remembered: calls = %q, %v", n.calls, err)
	}

	// After six hours the usual way is tried first again.
	*now = now.Add(botCheckMemory + time.Minute)
	n.flagged, n.calls = false, nil
	if _, err := withBotCheckFallback(ctx, n.attempt); err != nil || len(n.calls) != 1 || n.calls[0] != "" {
		t.Fatalf("after expiry: calls = %q, %v", n.calls, err)
	}
}

func TestBotCheckWithAVideoThatWontEmbed(t *testing.T) {
	resetBotCheck(t)
	ctx := context.Background()
	n := &network{flagged: true}

	// Neither way works: the bot check, with its advice, is what the user sees.
	_, err := withBotCheckFallback(ctx, n.attempt)
	if !isBotCheck(err) {
		t.Fatalf("err = %v", err)
	}

	// While remembered, a video that won't embed tries the usual way, which clears the
	// memory once the check is over.
	n.flagged, n.calls = false, nil
	if got, err := withBotCheckFallback(ctx, n.attempt); err != nil || got != "video via " || BotCheckActive() {
		t.Fatalf("got %q, %v; calls %q; active %v", got, err, n.calls, BotCheckActive())
	}
}

func TestBotCheckIsMarkedEvenWhenWindowsMangledIt(t *testing.T) {
	for _, stderr := range []string{
		"ERROR: [youtube] x: Sign in to confirm you're not a bot. Use --cookies-from-browser",
		"WARNING: HTTP Error 429: Too Many Requests\nERROR: [youtube] x: Sign in to confirm you�re not a bot.",
	} {
		if err := classifyFailure(stderr); !isBotCheck(err) {
			t.Errorf("%q not marked: %v", stderr, err)
		}
	}
	if err := classifyFailure("ERROR: HTTP Error 429: Too Many Requests"); isBotCheck(err) {
		t.Error("a plain 429 was marked as the bot check")
	}
}

func TestEmbeddedClientsReachYtdlp(t *testing.T) {
	job, _ := domain.NewJob("https://youtu.be/jNQXAC9IVRw", domain.Video720)
	joined := strings.Join(buildArgs(job, config.Config{YouTubeClients: EmbeddedClients}), " ")
	if !strings.Contains(joined, "--extractor-args youtube:player_client="+EmbeddedClients) {
		t.Errorf("args = %s", joined)
	}
	if joined := strings.Join(buildArgs(job, config.Config{}), " "); strings.Contains(joined, "player_client") {
		t.Errorf("usual args = %s", joined)
	}
}
