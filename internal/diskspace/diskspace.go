// Package diskspace reports free space on the drive holding a folder.
package diskspace

import "fmt"

// Free returns the bytes available to this user on the drive containing dir.
func Free(dir string) (uint64, error) {
	return free(dir)
}

// Format renders a byte count for messages, in decimal units like file managers use.
func Format(bytes uint64) string {
	const mb, gb = 1_000_000, 1_000_000_000
	if bytes >= gb {
		return fmt.Sprintf("%.1f GB", float64(bytes)/gb)
	}
	return fmt.Sprintf("%.0f MB", float64(bytes)/mb)
}
