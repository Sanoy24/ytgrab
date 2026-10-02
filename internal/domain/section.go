package domain

import (
	"errors"
	"fmt"
	"math"
)

var ErrInvalidSection = errors.New("Choose a start time before the end time, at least 1 second apart.")

// Section is the part of a video to download, in seconds from the start.
type Section struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// Valid accepts a section of at least one second within the first 24 hours.
func (s Section) Valid() bool {
	return !math.IsNaN(s.Start) && !math.IsNaN(s.End) &&
		s.Start >= 0 && s.End-s.Start >= 1 && s.End <= 24*60*60
}

// Label names the section for file names: "1m05s-2m30s", "5s-15s", "1h00m00s-1h02m10s".
func (s Section) Label() string {
	return clock(s.Start) + "-" + clock(s.End)
}

func clock(seconds float64) string {
	total := int(math.Round(seconds))
	h, m, sec := total/3600, total%3600/60, total%60
	switch {
	case h > 0:
		return fmt.Sprintf("%dh%02dm%02ds", h, m, sec)
	case m > 0:
		return fmt.Sprintf("%dm%02ds", m, sec)
	default:
		return fmt.Sprintf("%ds", sec)
	}
}
