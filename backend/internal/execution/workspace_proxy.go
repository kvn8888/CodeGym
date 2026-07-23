package execution

// WorkspaceProxy is the seam for long-lived practice workspaces (milestone
// 2+, not wired): a Monaco file-explorer sidebar in the frontend backed by a
// live sandbox's filesystem, plus a terminal. Maps directly onto the Daytona
// SDK: Sandbox.FileSystem (ListFiles/DownloadFile/UploadFile — paths are
// HOME-RELATIVE, same gotcha as the runner) and the SDK's PTY support.
//
// No routes exist for this yet. When they land, ListDir/ReadFile/WriteFile
// are plain HTTP, but OpenPTY needs a WebSocket upgrade — the current
// router/middleware chain must pass the upgrade through.

import (
	"context"
	"io"
)

type FileNode struct {
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size"`
}

type WorkspaceProxy interface {
	ListDir(ctx context.Context, workspaceID, path string) ([]FileNode, error)
	ReadFile(ctx context.Context, workspaceID, path string) ([]byte, error)
	WriteFile(ctx context.Context, workspaceID, path string, content []byte) error
	OpenPTY(ctx context.Context, workspaceID string) (PTYSession, error)
}

type PTYSession interface {
	io.ReadWriteCloser
	Resize(cols, rows int) error
}
