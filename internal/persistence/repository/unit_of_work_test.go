package repository

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

// TestWithinTx_CommitsOnNilError verifies the success path: when
// fn returns nil, WithinTx commits and the writes are visible in a
// subsequent transaction.
func TestWithinTx_CommitsOnNilError(t *testing.T) {
	resetDB(t)
	startsAt := time.Date(2026, 5, 22, 16, 0, 0, 0, time.UTC)

	ctx := context.Background()
	var savedID domain.AlertEventID
	if err := integration.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		got, err := uow.Alerts().SaveEvent(ctx, mustNewAlertEvent(t, "src", "fp-c", "canon-c", startsAt))
		if err != nil {
			return err
		}
		savedID = got.ID
		return nil
	}); err != nil {
		t.Fatalf("WithinTx: %v", err)
	}
	if savedID == 0 {
		t.Fatalf("savedID = 0, want non-zero")
	}

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		got, err := uow.Alerts().FindEventByID(ctx, savedID)
		if err != nil {
			t.Fatalf("FindEventByID: %v", err)
		}
		if got.ID != savedID {
			t.Errorf("got.ID = %d, want %d", got.ID, savedID)
		}
	})
}

// TestWithinTx_RollsBackOnError verifies the failure path: when fn
// returns a non-nil error, WithinTx rolls back and any writes done
// inside fn are not visible afterwards.
func TestWithinTx_RollsBackOnError(t *testing.T) {
	resetDB(t)
	startsAt := time.Date(2026, 5, 22, 17, 0, 0, 0, time.UTC)
	sentinel := errors.New("sentinel from fn")

	ctx := context.Background()
	var savedID domain.AlertEventID
	err := integration.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		got, ierr := uow.Alerts().SaveEvent(ctx, mustNewAlertEvent(t, "src", "fp-rb", "canon-rb", startsAt))
		if ierr != nil {
			return ierr
		}
		savedID = got.ID
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("WithinTx err = %v, want errors.Is sentinel", err)
	}
	if savedID == 0 {
		t.Fatalf("savedID = 0, want non-zero (the row WAS inserted in-tx, then rolled back)")
	}

	// Post-rollback: the row must not be visible. We hit the natural
	// key path because the auto-increment ID may or may not be
	// re-allocated by Postgres after a rollback (sequences do NOT
	// rewind), so checking by ID alone could be a false negative.
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		_, ferr := uow.Alerts().FindEventByNaturalKey(ctx, "src", "canon-rb", startsAt)
		if !errors.Is(ferr, domain.ErrNotFound) {
			t.Fatalf("FindEventByNaturalKey after rollback = %v, want errors.Is ErrNotFound", ferr)
		}
	})
}

// TestWithinTx_NestedRejected verifies the nested-WithinTx contract
// documented on UnitOfWorkFactory.WithinTx in ports/persistence.go:
// invoking WithinTx with a ctx that is already inside an active
// WithinTx boundary MUST return ports.ErrNestedTransaction without
// opening a second transaction or invoking the inner fn.
//
// The test layout is deliberately end-to-end:
//   - outer WithinTx writes one row using the inner-tx ctx;
//   - while still inside the outer fn, the test calls WithinTx
//     again with the very same ctx the outer received;
//   - we assert (a) the inner fn never runs, (b) the inner call
//     returned ErrNestedTransaction, and (c) the outer commit still
//     publishes the outer write.
//
// (c) is the load-bearing assertion: nested rejection must NOT poison
// the outer transaction. A bug that closed the outer tx on inner
// rejection would silently lose the outer write.
func TestWithinTx_NestedRejected(t *testing.T) {
	resetDB(t)
	startsAt := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)

	ctx := context.Background()
	var innerInvoked atomic.Bool
	var outerSavedID domain.AlertEventID

	err := integration.factory.WithinTx(ctx, func(outerCtx context.Context, uow ports.UnitOfWork) error {
		got, ierr := uow.Alerts().SaveEvent(outerCtx, mustNewAlertEvent(t, "src", "fp-nest", "canon-nest", startsAt))
		if ierr != nil {
			return ierr
		}
		outerSavedID = got.ID

		// Nested call on the same outerCtx: this must short-circuit
		// before opening a tx and before invoking the inner fn.
		nerr := integration.factory.WithinTx(outerCtx, func(_ context.Context, _ ports.UnitOfWork) error {
			innerInvoked.Store(true)
			return nil
		})
		if !errors.Is(nerr, ports.ErrNestedTransaction) {
			return fmt.Errorf("nested WithinTx err = %w, want errors.Is ErrNestedTransaction", nerr)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("outer WithinTx: %v", err)
	}
	if innerInvoked.Load() {
		t.Errorf("inner WithinTx fn was invoked; nesting must short-circuit before fn runs")
	}
	if outerSavedID == 0 {
		t.Fatalf("outer save did not produce an ID")
	}

	// The outer write MUST be visible after the outer commit:
	// nested rejection only short-circuits the inner call, never
	// poisons the outer transaction.
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		got, ferr := uow.Alerts().FindEventByID(ctx, outerSavedID)
		if ferr != nil {
			t.Fatalf("FindEventByID after nested-rejected commit: %v", ferr)
		}
		if got.ID != outerSavedID {
			t.Errorf("got.ID = %d, want %d", got.ID, outerSavedID)
		}
	})
}

// TestWithinTx_RollsBackOnPanicAndRePanics verifies the panic path:
// the transaction is rolled back AND the original panic value is
// propagated to the caller, preserving program-crash semantics.
func TestWithinTx_RollsBackOnPanicAndRePanics(t *testing.T) {
	resetDB(t)
	startsAt := time.Date(2026, 5, 22, 18, 0, 0, 0, time.UTC)

	ctx := context.Background()
	defer func() {
		v := recover()
		if v == nil {
			t.Fatal("expected panic to propagate, got none")
		}
		got, ok := v.(string)
		if !ok || got != "boom" {
			t.Errorf("panic value = %v (%T), want \"boom\"", v, v)
		}
		// Post-panic: the row must not be visible.
		withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
			_, ferr := uow.Alerts().FindEventByNaturalKey(ctx, "src", "canon-pn", startsAt)
			if !errors.Is(ferr, domain.ErrNotFound) {
				t.Errorf("FindEventByNaturalKey after panic = %v, want errors.Is ErrNotFound", ferr)
			}
		})
	}()

	_ = integration.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		if _, err := uow.Alerts().SaveEvent(ctx, mustNewAlertEvent(t, "src", "fp-pn", "canon-pn", startsAt)); err != nil {
			t.Fatalf("SaveEvent: %v", err)
		}
		panic("boom")
	})
	t.Fatal("unreachable: WithinTx must re-panic")
}

// TestUnitOfWork_BeginCommitAndRollback covers the explicit Begin
// entry point and verifies the close-state machine: after Commit,
// further Commit / Rollback returns errUoWClosed, and any repository
// method also returns the same closed-flag error rather than the
// driver-level "tx already committed".
func TestUnitOfWork_BeginCommitAndRollback(t *testing.T) {
	resetDB(t)
	startsAt := time.Date(2026, 5, 22, 19, 0, 0, 0, time.UTC)

	ctx := context.Background()

	// --- Commit path ---
	uow, err := integration.factory.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if _, err := uow.Alerts().SaveEvent(ctx, mustNewAlertEvent(t, "src", "fp-bc", "canon-bc", startsAt)); err != nil {
		_ = uow.Rollback(ctx)
		t.Fatalf("SaveEvent: %v", err)
	}
	if err := uow.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	// Double-Commit MUST be an error, not a panic.
	if err := uow.Commit(ctx); err == nil {
		t.Errorf("second Commit: want error, got nil")
	}
	// Repository call after Commit MUST return the closed-flag error
	// before reaching the driver. We reuse the per-tx repo handle on
	// purpose; production code should not, but the contract is that
	// such misuse is detected.
	if _, err := uow.Alerts().FindEventByNaturalKey(ctx, "src", "canon-bc", startsAt); err == nil {
		t.Errorf("FindEventByNaturalKey after Commit: want error, got nil")
	}

	// --- Rollback path ---
	uow2, err := integration.factory.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin (rollback path): %v", err)
	}
	if _, err := uow2.Alerts().SaveEvent(ctx, mustNewAlertEvent(t, "src", "fp-br", "canon-br", startsAt)); err != nil {
		t.Fatalf("SaveEvent (rollback path): %v", err)
	}
	if err := uow2.Rollback(ctx); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	// After Rollback the row must be gone.
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		_, ferr := uow.Alerts().FindEventByNaturalKey(ctx, "src", "canon-br", startsAt)
		if !errors.Is(ferr, domain.ErrNotFound) {
			t.Errorf("FindEventByNaturalKey after Rollback = %v, want ErrNotFound", ferr)
		}
	})
}
