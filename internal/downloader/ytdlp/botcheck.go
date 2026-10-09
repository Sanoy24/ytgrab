package ytdlp

import (
	"context"
	"sync"
	"time"
)

// YouTube's bot check ("Sign in to confirm you're not a bot") can cover a whole internet
// address, which on mobile and some home internet many people share, so it turns up
// without any downloading. YouTube's embedded players (on other sites and TVs) still get
// the full set of formats then, so when the check appears YTGrab switches to them for a
// while. Videos whose owners turned off embedding still need YouTube's usual way.
const (
	// EmbeddedClients are the yt-dlp player clients used while the bot check is on.
	EmbeddedClients = "tv_embedded,web_embedded"
	// botCheckMemory is how long YTGrab keeps using them before trying the usual way first.
	botCheckMemory = 6 * time.Hour
)

var botCheck struct {
	mu    sync.Mutex
	until time.Time
}

// botCheckNow is replaced in tests.
var botCheckNow = time.Now

// BotCheckActive reports whether YouTube's bot check was seen recently on this network.
func BotCheckActive() bool {
	botCheck.mu.Lock()
	defer botCheck.mu.Unlock()
	return botCheckNow().Before(botCheck.until)
}

func noteBotCheck() {
	botCheck.mu.Lock()
	defer botCheck.mu.Unlock()
	botCheck.until = botCheckNow().Add(botCheckMemory)
}

func clearBotCheck() {
	botCheck.mu.Lock()
	defer botCheck.mu.Unlock()
	botCheck.until = time.Time{}
}

func isBotCheck(err error) bool {
	failure, ok := err.(*Error)
	return ok && failure.botCheck
}

// clientArgs makes yt-dlp use the given YouTube player clients ("" for its own choice).
func clientArgs(clients string) []string {
	if clients == "" {
		return nil
	}
	return []string{"--extractor-args", "youtube:player_client=" + clients}
}

// withBotCheckFallback runs a YouTube request with yt-dlp's usual player clients ("") or,
// while the bot check is on, the embedded ones, and switches when the other way works.
func withBotCheckFallback[T any](ctx context.Context, attempt func(clients string) (T, error)) (T, error) {
	if BotCheckActive() {
		result, err := attempt(EmbeddedClients)
		if err == nil || ctx.Err() != nil {
			return result, err
		}
		// Not every video plays in embedded players; and the check may be over.
		usual, usualErr := attempt("")
		if usualErr == nil {
			clearBotCheck()
			return usual, nil
		}
		if isBotCheck(usualErr) {
			noteBotCheck()
			return usual, usualErr // the clearer of the two failures
		}
		return result, err
	}
	result, err := attempt("")
	if !isBotCheck(err) || ctx.Err() != nil {
		return result, err
	}
	noteBotCheck()
	if retried, retryErr := attempt(EmbeddedClients); retryErr == nil {
		return retried, nil
	}
	return result, err
}
