package benchmarkbudget

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var stateLocks sync.Map // absolute state path -> *sync.Mutex

// Store persists benchmark budget state in a JSON file. Each mutation takes a
// process-wide path lock plus a file lock, reloads the latest state, and writes
// atomically so separate command runs cannot overspend the same milestone.
type Store struct {
	path     string
	lockPath string
}

// Open loads or creates the budget state at path. priorSpendMicros is recorded
// only when the state file does not exist yet, so later runs preserve the
// explicit prior amount instead of resetting it.
func Open(path string, priorSpendMicros int64) (*Store, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("benchmark budget: state path is required")
	}
	if priorSpendMicros < 0 {
		return nil, ErrInvalidAmount
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("benchmark budget: resolve state path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
		return nil, fmt.Errorf("benchmark budget: create state directory: %w", err)
	}
	store := &Store{path: absolute, lockPath: absolute + ".lock"}
	if _, err := store.update(priorSpendMicros, func(*state) (bool, error) {
		return false, nil
	}); err != nil {
		return nil, err
	}
	return store, nil
}

// Snapshot returns the latest persisted budget state.
func (s *Store) Snapshot() (Snapshot, error) {
	if s == nil {
		return Snapshot{}, errors.New("benchmark budget: store is not configured")
	}
	return s.update(0, func(*state) (bool, error) {
		return false, nil
	})
}

// Reserve records a bounded maximum cost for an operation before work starts.
// Repeating the same operation ID with the same maximum is idempotent and
// never charges twice; a different maximum is rejected without changing state.
func (s *Store) Reserve(operationID string, maxCostMicros int64) (Snapshot, error) {
	operationID = strings.TrimSpace(operationID)
	if operationID == "" {
		return Snapshot{}, ErrInvalidOperationID
	}
	if maxCostMicros < 0 {
		return Snapshot{}, ErrInvalidAmount
	}
	return s.update(0, func(st *state) (bool, error) {
		if reservation, exists := st.Reservations[operationID]; exists {
			if reservation.MaxCostMicros == maxCostMicros {
				return false, nil
			}
			return false, fmt.Errorf("%w: operation %q reserved at %d, requested %d",
				ErrReservationConflict, operationID, reservation.MaxCostMicros, maxCostMicros)
		}
		committed, err := committedMicros(st)
		if err != nil {
			return false, err
		}
		if maxCostMicros > 0 && (committed >= st.LimitMicros || maxCostMicros > st.LimitMicros-committed) {
			return false, fmt.Errorf("%w: operation %q requests %d micro-USD with %d remaining",
				ErrBudgetExceeded, operationID, maxCostMicros, maxInt64(0, st.LimitMicros-committed))
		}
		st.Reservations[operationID] = Reservation{
			OperationID:   operationID,
			MaxCostMicros: maxCostMicros,
			Status:        ReservationReserved,
		}
		return true, nil
	})
}

// Settle records the actual cost for an operation, replacing its reserved
// maximum. Repeating the same settlement is a no-op; a different actual cost
// for an already-settled operation is rejected rather than double charged.
func (s *Store) Settle(operationID string, actualCostMicros int64) (Snapshot, error) {
	operationID = strings.TrimSpace(operationID)
	if operationID == "" {
		return Snapshot{}, ErrInvalidOperationID
	}
	if actualCostMicros < 0 {
		return Snapshot{}, ErrInvalidAmount
	}
	return s.update(0, func(st *state) (bool, error) {
		reservation, ok := st.Reservations[operationID]
		if !ok {
			return false, fmt.Errorf("%w: operation %q", ErrReservationNotFound, operationID)
		}
		if actualCostMicros > reservation.MaxCostMicros {
			return false, fmt.Errorf("%w: operation %q actual %d exceeds reserved %d",
				ErrSettlementExceedsReservation, operationID, actualCostMicros, reservation.MaxCostMicros)
		}
		if reservation.Status == ReservationSettled {
			if *reservation.ActualCostMicros == actualCostMicros {
				return false, nil
			}
			return false, fmt.Errorf("%w: operation %q settled at %d, got %d",
				ErrAlreadySettled, operationID, *reservation.ActualCostMicros, actualCostMicros)
		}
		actual := actualCostMicros
		reservation.ActualCostMicros = &actual
		reservation.Status = ReservationSettled
		st.Reservations[operationID] = reservation
		return true, nil
	})
}

// Reconcile settles an operation that was previously marked unknown. It has
// the same idempotent, no-double-charge semantics as Settle.
func (s *Store) Reconcile(operationID string, actualCostMicros int64) (Snapshot, error) {
	return s.Settle(operationID, actualCostMicros)
}

// MarkUnknown keeps an unsettled reservation fully committed until a later
// call to Settle or Reconcile records its actual cost.
func (s *Store) MarkUnknown(operationID string) (Snapshot, error) {
	operationID = strings.TrimSpace(operationID)
	if operationID == "" {
		return Snapshot{}, ErrInvalidOperationID
	}
	return s.update(0, func(st *state) (bool, error) {
		reservation, ok := st.Reservations[operationID]
		if !ok {
			return false, fmt.Errorf("%w: operation %q", ErrReservationNotFound, operationID)
		}
		switch reservation.Status {
		case ReservationUnknown:
			return false, nil
		case ReservationReserved:
			reservation.Status = ReservationUnknown
			reservation.ActualCostMicros = nil
			st.Reservations[operationID] = reservation
			return true, nil
		default:
			return false, fmt.Errorf("%w: operation %q is already settled", ErrAlreadySettled, operationID)
		}
	})
}

func (s *Store) update(priorSpendMicros int64, mutate func(*state) (bool, error)) (Snapshot, error) {
	if s == nil {
		return Snapshot{}, errors.New("benchmark budget: store is not configured")
	}
	pathLock := lockForPath(s.path)
	pathLock.Lock()
	defer pathLock.Unlock()

	lock, err := acquireFileLock(s.lockPath)
	if err != nil {
		return Snapshot{}, err
	}
	defer lock.Close()

	st, exists, err := loadState(s.path)
	if err != nil {
		return Snapshot{}, err
	}
	if !exists {
		if priorSpendMicros > MilestoneLimitMicros {
			return Snapshot{}, fmt.Errorf("%w: prior spending %d exceeds the %d micro-USD ceiling",
				ErrBudgetExceeded, priorSpendMicros, MilestoneLimitMicros)
		}
		st = newState(priorSpendMicros)
	}
	changed, err := mutate(st)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot, err := snapshotOf(st)
	if err != nil {
		return Snapshot{}, err
	}
	if snapshot.CommittedMicros > st.LimitMicros {
		return Snapshot{}, fmt.Errorf("%w: committed %d exceeds the %d micro-USD ceiling",
			ErrBudgetExceeded, snapshot.CommittedMicros, st.LimitMicros)
	}
	if !exists || changed {
		if err := saveState(s.path, st); err != nil {
			return Snapshot{}, err
		}
	}
	return snapshot, nil
}

func lockForPath(path string) *sync.Mutex {
	value, _ := stateLocks.LoadOrStore(path, &sync.Mutex{})
	return value.(*sync.Mutex)
}

func loadState(path string) (*state, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("benchmark budget: read state: %w", err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, false, errors.New("benchmark budget: state file is empty")
	}
	var st state
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, false, fmt.Errorf("benchmark budget: decode state: %w", err)
	}
	if err := validateState(&st); err != nil {
		return nil, false, err
	}
	return &st, true, nil
}

func saveState(path string, st *state) error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("benchmark budget: encode state: %w", err)
	}
	data = append(data, '\n')
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("benchmark budget: create temporary state: %w", err)
	}
	tempPath := temp.Name()
	cleanup := func() {
		_ = temp.Close()
		_ = os.Remove(tempPath)
	}
	if err := temp.Chmod(0o600); err != nil {
		cleanup()
		return fmt.Errorf("benchmark budget: set state permissions: %w", err)
	}
	if _, err := temp.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("benchmark budget: write state: %w", err)
	}
	if err := temp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("benchmark budget: sync state: %w", err)
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("benchmark budget: close state: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("benchmark budget: replace state: %w", err)
	}
	return nil
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
