package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestConfigRepo_SaveFindUpdateAndListAlertSourceProfile(t *testing.T) {
	resetDB(t)
	ctx := context.Background()
	var saved domain.AlertSourceProfile

	if err := integration.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		var err error
		saved, err = uow.Config().SaveAlertSourceProfile(ctx, mustNewAlertSourceProfile(t, "Primary Prometheus"))
		return err
	}); err != nil {
		t.Fatalf("save profile: %v", err)
	}
	if saved.ID == 0 || saved.CreatedAt.IsZero() || saved.UpdatedAt.IsZero() {
		t.Fatalf("saved profile missing generated fields: %+v", saved)
	}

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		got, err := uow.Config().FindAlertSourceProfileByID(ctx, saved.ID)
		if err != nil {
			t.Fatalf("FindAlertSourceProfileByID: %v", err)
		}
		if got.Name != "Primary Prometheus" || got.SecretRef != "secret/openclarion/prometheus" || !got.Enabled {
			t.Fatalf("got = %+v", got)
		}
	})

	updated := saved
	updated.Name = "Primary Prometheus Disabled"
	updated.AuthMode = domain.AlertSourceAuthModeNone
	updated.SecretRef = ""
	updated.Enabled = false
	updated.Labels = map[string]string{"env": "prod"}
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		got, err := uow.Config().UpdateAlertSourceProfile(ctx, updated)
		if err != nil {
			t.Fatalf("UpdateAlertSourceProfile: %v", err)
		}
		if got.SecretRef != "" || got.Enabled || got.Labels["env"] != "prod" {
			t.Fatalf("updated profile = %+v", got)
		}
	})

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		listed, err := uow.Config().ListAlertSourceProfiles(ctx, 10)
		if err != nil {
			t.Fatalf("ListAlertSourceProfiles: %v", err)
		}
		if len(listed) != 1 || listed[0].Name != "Primary Prometheus Disabled" {
			t.Fatalf("listed = %+v", listed)
		}
	})
}

func TestConfigRepo_AlertSourceProfileUniqueName(t *testing.T) {
	resetDB(t)

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		if _, err := uow.Config().SaveAlertSourceProfile(ctx, mustNewAlertSourceProfile(t, "Primary Prometheus")); err != nil {
			t.Fatalf("initial save: %v", err)
		}
	})

	err := integration.factory.WithinTx(context.Background(), func(ctx context.Context, uow ports.UnitOfWork) error {
		_, serr := uow.Config().SaveAlertSourceProfile(ctx, mustNewAlertSourceProfile(t, "Primary Prometheus"))
		return serr
	})
	if !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("duplicate save err = %v, want ErrAlreadyExists", err)
	}
}

func TestConfigRepo_AlertSourceProfileNotFoundAndInvalidInput(t *testing.T) {
	resetDB(t)

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		_, err := uow.Config().FindAlertSourceProfileByID(ctx, 404)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("find missing err = %v, want ErrNotFound", err)
		}
		_, err = uow.Config().FindAlertSourceProfileByID(ctx, 0)
		if !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("find zero err = %v, want ErrInvariantViolation", err)
		}
		_, err = uow.Config().ListAlertSourceProfiles(ctx, 0)
		if !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("list zero err = %v, want ErrInvariantViolation", err)
		}
		profile := mustNewAlertSourceProfile(t, "Missing")
		_, err = uow.Config().UpdateAlertSourceProfile(ctx, profile)
		if !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("update zero id err = %v, want ErrInvariantViolation", err)
		}
		profile.ID = 404
		_, err = uow.Config().UpdateAlertSourceProfile(ctx, profile)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("update missing err = %v, want ErrNotFound", err)
		}
	})
}

func TestConfigRepo_ListAlertSourceProfilesOrdersByUpdatedAt(t *testing.T) {
	resetDB(t)
	base := time.Date(2026, 6, 5, 3, 0, 0, 0, time.UTC)

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		oldest := mustNewAlertSourceProfile(t, "Oldest")
		oldest.UpdatedAt = base
		if _, err := uow.Config().SaveAlertSourceProfile(ctx, oldest); err != nil {
			t.Fatalf("save oldest: %v", err)
		}
		newest := mustNewAlertSourceProfile(t, "Newest")
		newest.BaseURL = "https://alertmanager.example.test"
		newest.Kind = domain.AlertSourceKindAlertmanager
		newest.UpdatedAt = base.Add(time.Minute)
		if _, err := uow.Config().SaveAlertSourceProfile(ctx, newest); err != nil {
			t.Fatalf("save newest: %v", err)
		}
	})

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		listed, err := uow.Config().ListAlertSourceProfiles(ctx, 1)
		if err != nil {
			t.Fatalf("ListAlertSourceProfiles: %v", err)
		}
		if len(listed) != 1 || listed[0].Name != "Newest" {
			t.Fatalf("listed = %+v, want Newest first", listed)
		}
	})
}

func mustNewAlertSourceProfile(t *testing.T, name string) domain.AlertSourceProfile {
	t.Helper()
	profile, err := domain.NewAlertSourceProfile(
		name,
		domain.AlertSourceKindPrometheus,
		"https://prometheus.example.test",
		domain.AlertSourceAuthModeBearer,
		"secret/openclarion/prometheus",
		true,
		map[string]string{"env": "test"},
	)
	if err != nil {
		t.Fatalf("NewAlertSourceProfile: %v", err)
	}
	return profile
}
