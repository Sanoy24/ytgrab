package domain

import (
	"testing"
	"time"
)

func TestWindow(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 10, 3, h, m, 0, 0, time.Local) }
	night, _ := ParseWindow("22-6")
	day, _ := ParseWindow("1-7")
	for _, c := range []struct {
		w    Window
		t    time.Time
		open bool
		next time.Time
	}{
		{night, at(23, 30), true, at(23, 30)},
		{night, at(3, 0), true, at(3, 0)},
		{night, at(6, 0), false, at(22, 0)},
		{night, at(12, 15), false, at(22, 0)},
		{day, at(0, 59), false, at(1, 0)},
		{day, at(7, 0), false, at(1, 0).AddDate(0, 0, 1)},
		{Window{}, at(12, 0), true, at(12, 0)},
	} {
		if got := c.w.Open(c.t); got != c.open {
			t.Errorf("%v open at %s = %v", c.w, c.t.Format("15:04"), got)
		}
		if got := c.w.NextOpen(c.t); !got.Equal(c.next) {
			t.Errorf("%v next after %s = %s, want %s", c.w, c.t.Format("15:04"), got, c.next)
		}
	}
	for _, bad := range []string{"1", "a-b", "-1-5", "1-24", "1-7-9"} {
		if _, ok := ParseWindow(bad); ok {
			t.Errorf("%q accepted", bad)
		}
	}
	if w, ok := ParseWindow("5-5"); !ok || !w.Always() || w.String() != "" {
		t.Error("equal hours mean any time")
	}
}
