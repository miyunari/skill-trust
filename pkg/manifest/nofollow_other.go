//go:build !unix

package manifest

import "os"

// openNoFollow is the portable fallback for platforms without O_NOFOLLOW
// (for example Windows). Symlinks encountered during the walk are still
// rejected by Create; this only narrows the walk-to-open race window on
// platforms that support the flag.
func openNoFollow(path string) (*os.File, error) {
	return os.Open(path)
}
