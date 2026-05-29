package repository

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

// makeGroupForEvidence inserts a fresh AlertGroup and returns its
// ID; EvidenceSnapshot has a NOT NULL FK on alert_group_id, so every
// evidence test needs a real group row to point at.
func makeGroupForEvidence(t *testing.T, key string) domain.AlertGroupID {
	t.Helper()
	first := time.Date(2026, 5, 22, 13, 0, 0, 0, time.UTC)
	last := first.Add(5 * time.Minute)
	var id domain.AlertGroupID
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		g, err := uow.Alerts().SaveGroup(ctx, mustNewAlertGroup(t, key, first, last))
		if err != nil {
			t.Fatalf("SaveGroup: %v", err)
		}
		id = g.ID
	})
	return id
}

func mustNewSnapshot(t *testing.T, groupID domain.AlertGroupID, digest string) domain.EvidenceSnapshot {
	t.Helper()
	s, err := domain.NewEvidenceSnapshot(
		groupID,
		digest,
		json.RawMessage(`{"metric":"cpu"}`),
		json.RawMessage(`{"providers":{"prom":"ok"}}`),
		domain.SnapshotStatusComplete,
		nil,
		"DiagnosisWorkflow",
	)
	if err != nil {
		t.Fatalf("NewEvidenceSnapshot: %v", err)
	}
	return s
}

func TestEvidenceRepository_SaveAndQuery(t *testing.T) {
	resetDB(t)
	groupID := makeGroupForEvidence(t, "evi-key-1")

	var saved domain.EvidenceSnapshot
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		got, err := uow.Evidence().Save(ctx, mustNewSnapshot(t, groupID, "digest-1"))
		if err != nil {
			t.Fatalf("Save: %v", err)
		}
		saved = got
	})
	if saved.ID == 0 {
		t.Errorf("saved.ID = 0, want non-zero")
	}
	if saved.CreatedAt.IsZero() {
		t.Errorf("saved.CreatedAt is zero")
	}

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		byID, err := uow.Evidence().FindByID(ctx, saved.ID)
		if err != nil {
			t.Fatalf("FindByID: %v", err)
		}
		if byID.Digest != "digest-1" {
			t.Errorf("FindByID.Digest = %q, want digest-1", byID.Digest)
		}
		byKey, err := uow.Evidence().FindByGroupAndDigest(ctx, groupID, "digest-1")
		if err != nil {
			t.Fatalf("FindByGroupAndDigest: %v", err)
		}
		if byKey.ID != saved.ID {
			t.Errorf("FindByGroupAndDigest.ID = %d, want %d", byKey.ID, saved.ID)
		}
	})
}

func TestEvidenceRepository_PerGroupDigestIdempotency(t *testing.T) {
	resetDB(t)
	groupA := makeGroupForEvidence(t, "evi-A")
	groupB := makeGroupForEvidence(t, "evi-B")

	// First write succeeds.
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		if _, err := uow.Evidence().Save(ctx, mustNewSnapshot(t, groupA, "shared-digest")); err != nil {
			t.Fatalf("Save groupA first: %v", err)
		}
	})

	// Same (group_id, digest) -> ErrAlreadyExists. This is the
	// per-group idempotency key documented on EvidenceSnapshot.
	ctx := context.Background()
	err := integration.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		_, serr := uow.Evidence().Save(ctx, mustNewSnapshot(t, groupA, "shared-digest"))
		return serr
	})
	if err == nil {
		t.Fatalf("Save (groupA, shared-digest) twice: want error, got nil")
	}
	if !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("Save (groupA, shared-digest) twice: want errors.Is ErrAlreadyExists, got %v", err)
	}

	// Different group, identical digest, distinct payload -> distinct
	// row. This proves the unique key is per-group and not global.
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		if _, err := uow.Evidence().Save(ctx, mustNewSnapshot(t, groupB, "shared-digest")); err != nil {
			t.Fatalf("Save groupB shared-digest: %v", err)
		}
	})
}

func TestEvidenceRepository_ListByGroup_OrdersByCreatedAtDesc(t *testing.T) {
	resetDB(t)
	groupID := makeGroupForEvidence(t, "evi-list")

	// Three snapshots with distinct digests; created_at is set by
	// the schema default (time.Now), so writing them in sequence
	// gives strictly increasing timestamps. ListByGroup sorts
	// descending so the most-recent snapshot appears first.
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		for _, d := range []string{"d-1", "d-2", "d-3"} {
			if _, err := uow.Evidence().Save(ctx, mustNewSnapshot(t, groupID, d)); err != nil {
				t.Fatalf("Save %s: %v", d, err)
			}
			// Ent's CreatedAt schema default has microsecond
			// resolution; sleep a hair to guarantee monotonic
			// ordering across the three rows.
			time.Sleep(2 * time.Millisecond)
		}
	})

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		out, err := uow.Evidence().ListByGroup(ctx, groupID, 10)
		if err != nil {
			t.Fatalf("ListByGroup: %v", err)
		}
		if len(out) != 3 {
			t.Fatalf("ListByGroup len = %d, want 3", len(out))
		}
		if out[0].Digest != "d-3" || out[1].Digest != "d-2" || out[2].Digest != "d-1" {
			t.Errorf("ListByGroup order = [%s,%s,%s], want [d-3,d-2,d-1]", out[0].Digest, out[1].Digest, out[2].Digest)
		}
	})
}

func TestEvidenceRepository_List_OrdersByCreatedAtDescAndLimit(t *testing.T) {
	resetDB(t)
	groupA := makeGroupForEvidence(t, "evi-list-all-A")
	groupB := makeGroupForEvidence(t, "evi-list-all-B")
	t0 := time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)

	var middle, newest domain.EvidenceSnapshot
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		oldest := mustNewSnapshot(t, groupA, "all-d-1")
		oldest.CreatedAt = t0.Add(10 * time.Minute)
		if _, err := uow.Evidence().Save(ctx, oldest); err != nil {
			t.Fatalf("Save oldest: %v", err)
		}

		middle = mustNewSnapshot(t, groupB, "all-d-2")
		middle.CreatedAt = t0.Add(20 * time.Minute)
		var err error
		middle, err = uow.Evidence().Save(ctx, middle)
		if err != nil {
			t.Fatalf("Save middle: %v", err)
		}

		newest = mustNewSnapshot(t, groupA, "all-d-3")
		newest.CreatedAt = t0.Add(30 * time.Minute)
		newest, err = uow.Evidence().Save(ctx, newest)
		if err != nil {
			t.Fatalf("Save newest: %v", err)
		}
	})

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		out, err := uow.Evidence().List(ctx, 2)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(out) != 2 {
			t.Fatalf("List len = %d, want 2", len(out))
		}
		if out[0].ID != newest.ID || out[1].ID != middle.ID {
			t.Errorf("List order = [%d,%d], want [%d,%d]", out[0].ID, out[1].ID, newest.ID, middle.ID)
		}
	})
}

func TestEvidenceRepository_List_RejectsBadLimit(t *testing.T) {
	resetDB(t)
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		_, err := uow.Evidence().List(ctx, 0)
		if !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("want errors.Is ErrInvariantViolation, got %v", err)
		}
	})
}
