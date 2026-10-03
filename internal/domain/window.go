package domain

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Window is the daily time when downloads may start, from hour Start up to hour End in
// local time. It may wrap past midnight (22–6). Start == End means any time.
type Window struct{ Start, End int }

// ParseWindow reads "" (any time) or "start-end" in whole hours, like "1-7".
func ParseWindow(value string) (Window, bool) {
	if value == "" {
		return Window{}, true
	}
	start, end, ok := strings.Cut(value, "-")
	if !ok {
		return Window{}, false
	}
	s, err1 := strconv.Atoi(start)
	e, err2 := strconv.Atoi(end)
	if err1 != nil || err2 != nil || s < 0 || s > 23 || e < 0 || e > 23 {
		return Window{}, false
	}
	return Window{Start: s, End: e}, true
}

func (w Window) String() string {
	if w.Always() {
		return ""
	}
	return fmt.Sprintf("%d-%d", w.Start, w.End)
}

// Always reports that downloads may start at any time.
func (w Window) Always() bool { return w.Start == w.End }

// Open reports whether downloads may start at t.
func (w Window) Open(t time.Time) bool {
	if w.Always() {
		return true
	}
	h := t.Hour()
	if w.Start < w.End {
		return h >= w.Start && h < w.End
	}
	return h >= w.Start || h < w.End // wraps past midnight
}

// NextOpen returns when the window next opens after t (t itself if it is open).
func (w Window) NextOpen(t time.Time) time.Time {
	if w.Open(t) {
		return t
	}
	next := time.Date(t.Year(), t.Month(), t.Day(), w.Start, 0, 0, 0, t.Location())
	if !next.After(t) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}
