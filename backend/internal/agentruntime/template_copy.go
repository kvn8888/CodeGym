package agentruntime

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
)

// copyTemplateDir replicates the file tree rooted at srcFS into dst,
// creating directories and copying regular files. Symbolic links are
// recreated where the platform allows; on Windows unprivileged creation
// fails, so failures there are skipped instead of aborting the build.
//
// The node_modules/.bin shim directory is skipped wholesale: it holds
// nothing but symlinks to package binaries, unreadable and unrecreatable
// without symlink privilege on stock Windows, and the agent runtime
// invokes tools through node directly rather than through those shims.
//
// Detection is behavior-based, not mode-based: some mounts report odd or
// empty mode bits for links, so a path is a link exactly when its target
// reads back, and regular exactly when its content does.
func copyTemplateDir(dst string, srcFS fs.FS) error {
	return fs.WalkDir(srcFS, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == "." {
			return nil
		}
		if entry.IsDir() && path == "node_modules/.bin" {
			return fs.SkipDir
		}
		target := filepath.Join(dst, filepath.FromSlash(path))
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if link, err := fs.ReadLink(srcFS, path); err == nil {
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := os.Symlink(link, target); err != nil {
				if runtime.GOOS == "windows" {
					return nil
				}
				return err
			}
			return nil
		}
		payload, err := fs.ReadFile(srcFS, path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, payload, 0o644)
	})
}
