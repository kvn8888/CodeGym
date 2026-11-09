//go:build !windows

package environmentverify

import (
	"os"
	"syscall"
)

// fileIdentity extracts device-plus-inode identity where the platform
// exposes it. Unix reports it through the stat structure; a FileInfo whose
// underlying value is not a stat result yields no identity, and callers then
// fall back to canonical path equality.
func fileIdentity(info os.FileInfo) (dev, ino uint64, ok bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, false
	}
	return uint64(stat.Dev), stat.Ino, true
}
