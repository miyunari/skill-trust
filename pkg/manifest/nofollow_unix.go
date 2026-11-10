//go:build unix

package manifest

import (
	"os"
	"syscall"
)

// openNoFollow opens path for reading, refusing to follow a symlink at
// the final path component. O_NONBLOCK is included so that a path that
// was a regular file during the walk but was replaced by a FIFO before
// the open fails fast instead of blocking until a writer appears; it has
// no effect on regular files.
func openNoFollow(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
}
