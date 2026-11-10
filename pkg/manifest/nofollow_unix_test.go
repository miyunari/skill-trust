//go:build unix

package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

// TestOpenNoFollowRefusesSymlink verifies the walk-to-open race guard: a
// path that was a regular file during the walk but is a symlink by the
// time of the open must fail to open rather than hash the link target.
func TestOpenNoFollowRefusesSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.txt")
	if err := os.WriteFile(target, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	if f, err := openNoFollow(link); err == nil {
		f.Close()
		t.Fatal("expected openNoFollow to refuse a symlink")
	}

	f, err := openNoFollow(target)
	if err != nil {
		t.Fatalf("openNoFollow(regular file): %v", err)
	}
	f.Close()
}
