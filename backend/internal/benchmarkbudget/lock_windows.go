//go:build windows

package benchmarkbudget

// Windows has no cross-process lock in this package's standard-library-only
// implementation. Persistent enforcement is rejected explicitly instead of
// silently providing weaker process-local guarantees. The budget CLI therefore
// cannot persist or mutate budget state on Windows until a real file lock is
// added.
type fileLock struct{}

func acquireFileLock(path string) (*fileLock, error) {
	_ = path
	return nil, ErrPersistentLockUnsupported
}

func (l *fileLock) Close() error {
	return nil
}
