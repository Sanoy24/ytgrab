package config

import "testing"

func TestValidateListenAddress(t *testing.T) {
	tests := []struct {
		address string
		wantErr bool
	}{
		{"127.0.0.1:8787", false},
		{"[::1]:8787", false},
		{"0.0.0.0:8787", true},
		{"192.168.1.20:8787", true},
		{":8787", true},
		{"127.0.0.1:0", true},
		{"127.0.0.1:65536", true},
	}
	for _, tc := range tests {
		t.Run(tc.address, func(t *testing.T) {
			err := (Config{ListenAddress: tc.address, DataDir: t.TempDir(), DownloadsDir: t.TempDir(), ShutdownTimeout: 1}).Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate() error = %v, want error %v", err, tc.wantErr)
			}
		})
	}
}
