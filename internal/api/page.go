package api

import (
	"sync"
	"time"
)

// Only an open YTGrab page holds progress streams, so they tell whether someone is
// watching the queue in a browser.
var streams struct {
	sync.Mutex
	open       int
	lastClosed time.Time
}

func streamOpened() {
	streams.Lock()
	streams.open++
	streams.Unlock()
}

func streamClosed() {
	streams.Lock()
	streams.open--
	streams.lastClosed = time.Now()
	streams.Unlock()
}

// PageOpen reports whether a YTGrab page is showing download progress, or was moments
// ago: a stream ends when its download does, just before the page would announce it.
func PageOpen() bool {
	streams.Lock()
	defer streams.Unlock()
	return streams.open > 0 || time.Since(streams.lastClosed) < 10*time.Second
}
