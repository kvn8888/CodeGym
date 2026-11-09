//go:build windows

package environmentverify

import "os"

// fileIdentity reports no device-inode identity on Windows, which exposes no
// equivalent in its FileInfo. Callers therefore rely on canonical path
// equality for same-file detection — correct for the artifact tree Windows
// presents, and never a false match.
func fileIdentity(os.FileInfo) (dev, ino uint64, ok bool) {
	return 0, 0, false
}
