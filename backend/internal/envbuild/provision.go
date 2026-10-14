package envbuild

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

// HostProvisioner provisions isolated temporary directories. It is the
// first WorkspaceProvisioner behind the seam: same isolation grade as the
// accepted benchmark runs, replaceable by a Daytona provisioner when relay
// reachability lands without changing callers.
type HostProvisioner struct{}

// Provision creates an isolated working directory. Labels are accepted for
// interface parity and future provisioners; the host directory carries none.
func (HostProvisioner) Provision(context.Context, map[string]string) (Workspace, error) {
	root, err := os.MkdirTemp("", "codegym-envbuild-")
	if err != nil {
		return Workspace{}, err
	}
	return Workspace{ID: filepath.Base(root), Root: root}, nil
}

// Destroy removes the workspace directory. It refuses a blank root rather
// than risk a destructive call.
func (HostProvisioner) Destroy(_ context.Context, workspace Workspace) error {
	if workspace.Root == "" {
		return errors.New("envbuild: refuse to destroy a workspace with no root")
	}
	return os.RemoveAll(workspace.Root)
}
