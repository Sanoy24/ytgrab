package tray

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Sanoy24/ytgrab/internal/activity"
)

func TestRunReturnsStartErrorWithoutIcon(t *testing.T) {
	want := errors.New("port in use")
	err := Run(context.Background(), Actions{Open: func(string) { t.Error("Open called") }},
		func(context.Context, Hooks) error { return want })
	if !errors.Is(err, want) {
		t.Fatalf("Run = %v, want %v", err, want)
	}
}

func TestNotifications(t *testing.T) {
	cases := []struct {
		summary activity.Summary
		want    []note
	}{
		{activity.Summary{Downloading: 1}, nil},
		{activity.Summary{Finished: []string{"Me at the zoo"}}, []note{{"Download finished", "Me at the zoo"}}},
		{activity.Summary{Finished: []string{"A", "B", "C", "D", "E"}}, []note{{"5 downloads finished", "A\nB\nC\nand 2 more"}}},
		{activity.Summary{
			Finished: []string{"A", "B"},
			Failed:   []activity.Failure{{Title: "C", Message: "This video is private."}},
		}, []note{{"2 downloads finished", "A\nB"}, {"Download failed", "C\nThis video is private."}}},
		{activity.Summary{Failed: []activity.Failure{{Title: "C"}, {Title: "D"}}}, []note{{"2 downloads failed", "C\nD\nOpen YTGrab to retry them."}}},
	}
	for _, c := range cases {
		if got := notifications(c.summary); !reflect.DeepEqual(got, c.want) {
			t.Errorf("notifications(%+v) = %q, want %q", c.summary, got, c.want)
		}
	}
}
