//go:build windows

package benchmarkbudget

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestPersistentBudgetRejectsUnsupportedWindowsLocking(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "budget-state.json"), 0)
	if !errors.Is(err, ErrPersistentLockUnsupported) {
		t.Fatalf("Open = %v, want ErrPersistentLockUnsupported", err)
	}
	if store != nil {
		t.Fatalf("store = %+v, want nil when persistent enforcement is unsupported", store)
	}
}
