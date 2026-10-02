package diskspace

import "testing"

func TestFreeReportsSpaceOnARealFolder(t *testing.T) {
	free, err := Free(t.TempDir())
	if err != nil || free == 0 {
		t.Fatalf("Free = %d, %v", free, err)
	}
	if _, err := Free(t.TempDir() + "/missing/folder"); err == nil {
		t.Fatal("a missing folder should be an error")
	}
}

func TestFormat(t *testing.T) {
	for bytes, want := range map[uint64]string{
		120_000_000:   "120 MB",
		5_400_000_000: "5.4 GB",
		900_000:       "1 MB",
	} {
		if got := Format(bytes); got != want {
			t.Errorf("Format(%d) = %q, want %q", bytes, got, want)
		}
	}
}
