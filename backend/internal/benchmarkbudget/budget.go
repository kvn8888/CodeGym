// Package benchmarkbudget persists the local problem-quality benchmark's
// cumulative $30 milestone budget. It deliberately provides accounting and
// reservation enforcement only; paid adapters must call Reserve before an
// operation and Settle or MarkUnknown afterwards.
package benchmarkbudget

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/kvn8888/codegym/backend/internal/usage"
)

const (
	// Currency is the only currency the milestone budget accounts in.
	Currency = "USD"
	// MilestoneLimitMicros is the $30 cumulative milestone ceiling in micro-USD.
	MilestoneLimitMicros int64 = 30 * usage.MicrosPerUSD
	stateVersion               = 1
)

var (
	ErrInvalidOperationID           = errors.New("benchmark budget: operation ID is required")
	ErrInvalidAmount                = errors.New("benchmark budget: amount must be non-negative")
	ErrInvalidLimit                 = errors.New("benchmark budget: state limit does not match the fixed milestone ceiling")
	ErrBudgetExceeded               = errors.New("benchmark budget: reservation exceeds remaining allowance")
	ErrReservationConflict          = errors.New("benchmark budget: operation is already reserved with a different maximum")
	ErrReservationNotFound          = errors.New("benchmark budget: reservation not found")
	ErrSettlementExceedsReservation = errors.New("benchmark budget: actual cost exceeds reserved maximum")
	ErrAlreadySettled               = errors.New("benchmark budget: reservation is already settled with a different cost")
	ErrAccountingOverflow           = errors.New("benchmark budget: accounting arithmetic overflow")
	ErrPersistentLockUnsupported    = errors.New("benchmark budget: persistent cross-process locking is unsupported on this platform")
)

// ReservationStatus tracks whether a reserved maximum is still outstanding or
// has been settled to an actual cost. Unknown costs stay reserved until
// reconciled, so they keep counting against the milestone ceiling.
type ReservationStatus string

const (
	ReservationReserved = ReservationStatus("reserved")
	ReservationUnknown  = ReservationStatus("unknown")
	ReservationSettled  = ReservationStatus("settled")
)

// Reservation is one operation's bounded cost reservation.
type Reservation struct {
	OperationID      string            `json:"operation_id"`
	MaxCostMicros    int64             `json:"max_cost_micros"`
	ActualCostMicros *int64            `json:"actual_cost_micros,omitempty"`
	Status           ReservationStatus `json:"status"`
}

// Snapshot is the reportable budget state. All money values are integer
// micro-USD; SettledMicros counts only reconciled actual costs, while
// ReservedMicros includes both open reservations and unknown costs that remain
// reserved.
type Snapshot struct {
	Currency         string        `json:"currency"`
	LimitMicros      int64         `json:"limit_micros"`
	PriorSpendMicros int64         `json:"prior_spend_micros"`
	ReservedMicros   int64         `json:"reserved_micros"`
	SettledMicros    int64         `json:"settled_micros"`
	CommittedMicros  int64         `json:"committed_micros"`
	RemainingMicros  int64         `json:"remaining_micros"`
	Reservations     []Reservation `json:"reservations"`
}

type state struct {
	Version          int                    `json:"version"`
	Currency         string                 `json:"currency"`
	LimitMicros      int64                  `json:"limit_micros"`
	PriorSpendMicros int64                  `json:"prior_spend_micros"`
	Reservations     map[string]Reservation `json:"reservations"`
}

func newState(priorSpendMicros int64) *state {
	return &state{
		Version:          stateVersion,
		Currency:         Currency,
		LimitMicros:      MilestoneLimitMicros,
		PriorSpendMicros: priorSpendMicros,
		Reservations:     map[string]Reservation{},
	}
}

func validateState(st *state) error {
	if st.Version != stateVersion {
		return fmt.Errorf("benchmark budget: unsupported state version %d", st.Version)
	}
	if st.Currency != Currency {
		return fmt.Errorf("benchmark budget: unsupported currency %q", st.Currency)
	}
	if st.LimitMicros != MilestoneLimitMicros {
		return fmt.Errorf("%w: got %d, want %d", ErrInvalidLimit, st.LimitMicros, MilestoneLimitMicros)
	}
	if st.PriorSpendMicros < 0 {
		return errors.New("benchmark budget: prior spending must be non-negative")
	}
	if st.PriorSpendMicros > st.LimitMicros {
		return fmt.Errorf("%w: prior spending %d exceeds the %d micro-USD ceiling",
			ErrBudgetExceeded, st.PriorSpendMicros, st.LimitMicros)
	}
	if st.Reservations == nil {
		st.Reservations = map[string]Reservation{}
	}
	for operationID, reservation := range st.Reservations {
		if strings.TrimSpace(operationID) == "" || operationID != reservation.OperationID {
			return fmt.Errorf("benchmark budget: reservation key %q does not match operation ID %q", operationID, reservation.OperationID)
		}
		if reservation.MaxCostMicros < 0 {
			return fmt.Errorf("benchmark budget: reservation %q has a negative maximum", operationID)
		}
		switch reservation.Status {
		case ReservationReserved, ReservationUnknown:
			if reservation.ActualCostMicros != nil {
				return fmt.Errorf("benchmark budget: unsettled reservation %q has an actual cost", operationID)
			}
		case ReservationSettled:
			if reservation.ActualCostMicros == nil || *reservation.ActualCostMicros < 0 ||
				*reservation.ActualCostMicros > reservation.MaxCostMicros {
				return fmt.Errorf("benchmark budget: settled reservation %q has an invalid actual cost", operationID)
			}
		default:
			return fmt.Errorf("benchmark budget: reservation %q has unknown status %q", operationID, reservation.Status)
		}
	}
	snapshot, err := snapshotOf(st)
	if err != nil {
		return err
	}
	if snapshot.CommittedMicros > st.LimitMicros {
		return fmt.Errorf("%w: committed %d exceeds the %d micro-USD ceiling",
			ErrBudgetExceeded, snapshot.CommittedMicros, st.LimitMicros)
	}
	return nil
}

func snapshotOf(st *state) (Snapshot, error) {
	operationIDs := make([]string, 0, len(st.Reservations))
	for operationID := range st.Reservations {
		operationIDs = append(operationIDs, operationID)
	}
	sort.Strings(operationIDs)

	reservations := make([]Reservation, 0, len(operationIDs))
	var reservedMicros, settledMicros int64
	for _, operationID := range operationIDs {
		reservation := copyReservation(st.Reservations[operationID])
		reservations = append(reservations, reservation)
		var err error
		if reservation.Status == ReservationSettled {
			settledMicros, err = addMicros(settledMicros, *reservation.ActualCostMicros)
			if err != nil {
				return Snapshot{}, err
			}
			continue
		}
		reservedMicros, err = addMicros(reservedMicros, reservation.MaxCostMicros)
		if err != nil {
			return Snapshot{}, err
		}
	}
	committedMicros, err := addMicros(st.PriorSpendMicros, reservedMicros)
	if err != nil {
		return Snapshot{}, err
	}
	committedMicros, err = addMicros(committedMicros, settledMicros)
	if err != nil {
		return Snapshot{}, err
	}
	remainingMicros := st.LimitMicros - committedMicros
	if remainingMicros < 0 {
		remainingMicros = 0
	}
	return Snapshot{
		Currency:         st.Currency,
		LimitMicros:      st.LimitMicros,
		PriorSpendMicros: st.PriorSpendMicros,
		ReservedMicros:   reservedMicros,
		SettledMicros:    settledMicros,
		CommittedMicros:  committedMicros,
		RemainingMicros:  remainingMicros,
		Reservations:     reservations,
	}, nil
}

func copyReservation(reservation Reservation) Reservation {
	if reservation.ActualCostMicros != nil {
		actual := *reservation.ActualCostMicros
		reservation.ActualCostMicros = &actual
	}
	return reservation
}

func committedMicros(st *state) (int64, error) {
	snapshot, err := snapshotOf(st)
	if err != nil {
		return 0, err
	}
	return snapshot.CommittedMicros, nil
}

func addMicros(left, right int64) (int64, error) {
	if right > 0 && left > math.MaxInt64-right {
		return 0, ErrAccountingOverflow
	}
	return left + right, nil
}
