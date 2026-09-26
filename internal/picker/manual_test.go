package picker

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestManualPick opens the real folder window. Run it only on a desktop:
//
//	YTGRAB_MANUAL_PICKER=1 go test ./internal/picker -run TestManualPick -v
func TestManualPick(t *testing.T) {
	if os.Getenv("YTGRAB_MANUAL_PICKER") == "" {
		t.Skip("set YTGRAB_MANUAL_PICKER=1 to open the real folder window")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	started := time.Now()
	path, err := New().Pick(ctx, os.Getenv("USERPROFILE"))
	t.Logf("Pick returned %q, %v after %v", path, err, time.Since(started).Round(time.Millisecond))
}
