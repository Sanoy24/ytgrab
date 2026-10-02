package tray

import (
	"context"
	"errors"
	"testing"
)

func TestRunReturnsStartErrorWithoutIcon(t *testing.T) {
	want := errors.New("port in use")
	err := Run(context.Background(), Actions{Open: func(string) { t.Error("Open called") }},
		func(context.Context, func(string)) error { return want })
	if !errors.Is(err, want) {
		t.Fatalf("Run = %v, want %v", err, want)
	}
}
