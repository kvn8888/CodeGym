package benchmarkbudget

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
)

func newTestStore(t *testing.T, priorSpendMicros int64) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "budget-state.json")
	store, err := Open(path, priorSpendMicros)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return store, path
}

func mustSnapshot(t *testing.T, store *Store) Snapshot {
	t.Helper()
	snapshot, err := store.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	return snapshot
}

func TestReserveEnforcesMilestoneLimit(t *testing.T) {
	store, _ := newTestStore(t, 0)
	if _, err := store.Reserve("first", MilestoneLimitMicros-1_000_000); err != nil {
		t.Fatalf("Reserve first: %v", err)
	}
	if _, err := store.Reserve("too-large", 2_000_000); !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("Reserve too-large = %v, want ErrBudgetExceeded", err)
	}
	snapshot := mustSnapshot(t, store)
	if snapshot.CommittedMicros != MilestoneLimitMicros-1_000_000 ||
		snapshot.RemainingMicros != 1_000_000 || len(snapshot.Reservations) != 1 {
		t.Fatalf("snapshot after rejection = %+v, want rejected reservation not persisted", snapshot)
	}
	if _, err := store.Reserve("exact-remainder", 1_000_000); err != nil {
		t.Fatalf("Reserve exact remainder: %v", err)
	}
	snapshot = mustSnapshot(t, store)
	if snapshot.CommittedMicros != MilestoneLimitMicros || snapshot.RemainingMicros != 0 {
		t.Fatalf("snapshot at limit = %+v", snapshot)
	}
	if _, err := store.Reserve("over-limit", 1); !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("Reserve over-limit = %v, want ErrBudgetExceeded", err)
	}
}

func TestPriorSpendingCountsTowardLimitAndPersists(t *testing.T) {
	prior := int64(29_000_000)
	store, path := newTestStore(t, prior)
	snapshot := mustSnapshot(t, store)
	if snapshot.PriorSpendMicros != prior || snapshot.CommittedMicros != prior ||
		snapshot.RemainingMicros != MilestoneLimitMicros-prior {
		t.Fatalf("snapshot with prior spending = %+v", snapshot)
	}
	if _, err := store.Reserve("too-large", 2_000_000); !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("Reserve with prior spending = %v, want ErrBudgetExceeded", err)
	}
	if _, err := store.Reserve("within-remaining", 1_000_000); err != nil {
		t.Fatalf("Reserve within remaining: %v", err)
	}

	reopened, err := Open(path, 99_000_000)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	snapshot = mustSnapshot(t, reopened)
	if snapshot.PriorSpendMicros != prior || snapshot.CommittedMicros != MilestoneLimitMicros {
		t.Fatalf("reopened snapshot = %+v, want persisted prior spending preserved", snapshot)
	}
}

func TestSettleReplacesReservationWithoutDoubleCharging(t *testing.T) {
	store, _ := newTestStore(t, 0)
	if _, err := store.Reserve("operation", 5_000_000); err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	snapshot, err := store.Settle("operation", 2_000_000)
	if err != nil {
		t.Fatalf("Settle: %v", err)
	}
	if snapshot.ReservedMicros != 0 || snapshot.SettledMicros != 2_000_000 ||
		snapshot.CommittedMicros != 2_000_000 || snapshot.RemainingMicros != MilestoneLimitMicros-2_000_000 {
		t.Fatalf("settled snapshot = %+v", snapshot)
	}
	again, err := store.Settle("operation", 2_000_000)
	if err != nil {
		t.Fatalf("repeated Settle: %v", err)
	}
	if again.CommittedMicros != 2_000_000 || again.SettledMicros != 2_000_000 {
		t.Fatalf("repeated settlement double charged: %+v", again)
	}
	if _, err := store.Settle("operation", 3_000_000); !errors.Is(err, ErrAlreadySettled) {
		t.Fatalf("Settle different actual = %v, want ErrAlreadySettled", err)
	}
	snapshot = mustSnapshot(t, store)
	if snapshot.CommittedMicros != 2_000_000 {
		t.Fatalf("snapshot after rejected settlement = %+v", snapshot)
	}
	if _, err := store.Settle("missing", 1); !errors.Is(err, ErrReservationNotFound) {
		t.Fatalf("Settle missing = %v, want ErrReservationNotFound", err)
	}
	if _, err := store.Settle("operation", -1); !errors.Is(err, ErrInvalidAmount) {
		t.Fatalf("Settle negative = %v, want ErrInvalidAmount", err)
	}
	if _, err := store.Reserve("over-settlement", 1_000_000); err != nil {
		t.Fatalf("Reserve over-settlement: %v", err)
	}
	if _, err := store.Settle("over-settlement", 1_000_001); !errors.Is(err, ErrSettlementExceedsReservation) {
		t.Fatalf("Settle above reservation = %v, want ErrSettlementExceedsReservation", err)
	}
}

func TestUnknownCostsRemainReservedUntilReconciled(t *testing.T) {
	store, _ := newTestStore(t, 0)
	if _, err := store.Reserve("unknown-operation", 4_000_000); err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	snapshot, err := store.MarkUnknown("unknown-operation")
	if err != nil {
		t.Fatalf("MarkUnknown: %v", err)
	}
	if snapshot.ReservedMicros != 4_000_000 || snapshot.SettledMicros != 0 ||
		snapshot.CommittedMicros != 4_000_000 || snapshot.Reservations[0].Status != ReservationUnknown {
		t.Fatalf("unknown snapshot = %+v, want full maximum still reserved", snapshot)
	}
	if _, err := store.Reserve("cannot-fit", MilestoneLimitMicros-3_000_000); !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("Reserve while unknown = %v, want ErrBudgetExceeded", err)
	}
	snapshot, err = store.Reconcile("unknown-operation", 3_000_000)
	if err != nil {
		t.Fatalf("reconcile unknown: %v", err)
	}
	if snapshot.ReservedMicros != 0 || snapshot.SettledMicros != 3_000_000 ||
		snapshot.CommittedMicros != 3_000_000 || snapshot.Reservations[0].Status != ReservationSettled {
		t.Fatalf("reconciled snapshot = %+v", snapshot)
	}
	if _, err := store.MarkUnknown("unknown-operation"); !errors.Is(err, ErrAlreadySettled) {
		t.Fatalf("MarkUnknown after settlement = %v, want ErrAlreadySettled", err)
	}
}

func TestRepeatedOperationIDsAreSafe(t *testing.T) {
	store, _ := newTestStore(t, 0)
	if _, err := store.Reserve("duplicate", 5_000_000); err != nil {
		t.Fatalf("first Reserve: %v", err)
	}
	snapshot, err := store.Reserve("duplicate", 5_000_000)
	if err != nil {
		t.Fatalf("duplicate Reserve: %v", err)
	}
	if snapshot.CommittedMicros != 5_000_000 || len(snapshot.Reservations) != 1 {
		t.Fatalf("duplicate reservation double charged: %+v", snapshot)
	}
	if _, err := store.Settle("duplicate", 5_000_000); err != nil {
		t.Fatalf("Settle: %v", err)
	}
	if _, err := store.Reserve("duplicate", 10_000_000); !errors.Is(err, ErrReservationConflict) {
		t.Fatalf("Reserve after settlement with a different maximum = %v, want ErrReservationConflict", err)
	}
	snapshot = mustSnapshot(t, store)
	if snapshot.CommittedMicros != 5_000_000 || snapshot.Reservations[0].Status != ReservationSettled ||
		snapshot.Reservations[0].MaxCostMicros != 5_000_000 {
		t.Fatalf("conflicting reservation changed state: %+v", snapshot)
	}
}

func TestReserveConflictingMaximumBeforeSettlementDoesNotChangeState(t *testing.T) {
	store, _ := newTestStore(t, 0)
	if _, err := store.Reserve("operation", 5_000_000); err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if _, err := store.Reserve("operation", 6_000_000); !errors.Is(err, ErrReservationConflict) {
		t.Fatalf("conflicting Reserve = %v, want ErrReservationConflict", err)
	}
	snapshot := mustSnapshot(t, store)
	if snapshot.CommittedMicros != 5_000_000 || snapshot.ReservedMicros != 5_000_000 ||
		len(snapshot.Reservations) != 1 || snapshot.Reservations[0].MaxCostMicros != 5_000_000 {
		t.Fatalf("snapshot after conflict = %+v, want original reservation intact", snapshot)
	}
}

func TestOpenRejectsPersistedStateThatDoesNotUseFixedCeiling(t *testing.T) {
	path := filepath.Join(t.TempDir(), "budget-state.json")
	st := newState(0)
	st.LimitMicros = MilestoneLimitMicros - 1
	writeState(t, path, st)
	if _, err := Open(path, 0); !errors.Is(err, ErrInvalidLimit) {
		t.Fatalf("Open with invalid persisted limit = %v, want ErrInvalidLimit", err)
	}
}

func TestOpenRejectsPriorSpendingAboveFixedCeiling(t *testing.T) {
	path := filepath.Join(t.TempDir(), "budget-state.json")
	if _, err := Open(path, MilestoneLimitMicros+1); !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("Open with prior spending above ceiling = %v, want ErrBudgetExceeded", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("state file exists after rejected prior spending: %v", err)
	}
}

func TestOpenRejectsPersistedCommitmentAboveFixedCeiling(t *testing.T) {
	path := filepath.Join(t.TempDir(), "budget-state.json")
	st := newState(MilestoneLimitMicros)
	st.Reservations["over"] = Reservation{
		OperationID:   "over",
		MaxCostMicros: 1,
		Status:        ReservationReserved,
	}
	writeState(t, path, st)
	if _, err := Open(path, 0); !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("Open with commitment above ceiling = %v, want ErrBudgetExceeded", err)
	}
}

func TestOpenDetectsPersistedAccountingOverflow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "budget-state.json")
	st := newState(0)
	for _, operationID := range []string{"first", "second"} {
		st.Reservations[operationID] = Reservation{
			OperationID:   operationID,
			MaxCostMicros: math.MaxInt64,
			Status:        ReservationReserved,
		}
	}
	writeState(t, path, st)
	if _, err := Open(path, 0); !errors.Is(err, ErrAccountingOverflow) {
		t.Fatalf("Open with overflowing reservations = %v, want ErrAccountingOverflow", err)
	}
}

func writeState(t *testing.T, path string, st *state) {
	t.Helper()
	data, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestStatePersistsAcrossStoreInstances(t *testing.T) {
	store, path := newTestStore(t, 2_000_000)
	if _, err := store.Reserve("persisted", 6_000_000); err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if _, err := store.Settle("persisted", 4_000_000); err != nil {
		t.Fatalf("Settle: %v", err)
	}

	reopened, err := Open(path, 0)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	snapshot := mustSnapshot(t, reopened)
	if snapshot.PriorSpendMicros != 2_000_000 || snapshot.CommittedMicros != 6_000_000 ||
		snapshot.SettledMicros != 4_000_000 || len(snapshot.Reservations) != 1 {
		t.Fatalf("reopened snapshot = %+v", snapshot)
	}
}

func TestConcurrentReservationsCannotOverspend(t *testing.T) {
	path := filepath.Join(t.TempDir(), "budget-state.json")
	storeA, err := Open(path, 0)
	if err != nil {
		t.Fatalf("Open A: %v", err)
	}
	storeB, err := Open(path, 0)
	if err != nil {
		t.Fatalf("Open B: %v", err)
	}
	const attempts = 80
	var wg sync.WaitGroup
	var mu sync.Mutex
	successes, exceeded, other := 0, 0, 0
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		store := storeA
		if i%2 == 1 {
			store = storeB
		}
		go func(i int, store *Store) {
			defer wg.Done()
			_, err := store.Reserve("operation-"+strconv.Itoa(i), 1_000_000)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				successes++
			case errors.Is(err, ErrBudgetExceeded):
				exceeded++
			default:
				other++
			}
		}(i, store)
	}
	wg.Wait()
	if other != 0 {
		t.Fatalf("unexpected reservation errors = %d", other)
	}
	if successes != 30 || exceeded != attempts-30 {
		t.Fatalf("reservations succeeded=%d exceeded=%d, want 30/%d", successes, exceeded, attempts-30)
	}
	snapshot := mustSnapshot(t, storeA)
	if snapshot.CommittedMicros != MilestoneLimitMicros || snapshot.RemainingMicros != 0 ||
		len(snapshot.Reservations) != 30 {
		t.Fatalf("concurrent snapshot = %+v", snapshot)
	}
}
