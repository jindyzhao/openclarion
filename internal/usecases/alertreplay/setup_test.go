package alertreplay_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/openclarion/openclarion/internal/persistence/ent"
	"github.com/openclarion/openclarion/internal/persistence/repository"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

// alertreplay needs a live Postgres for its integration tests
// because the safety-valve / refresh / close-on-snapshot paths only
// surface end-to-end. The harness mirrors the repository and
// alertingest packages -- a private container per package keeps
// cross-package fixtures from leaking the integration / resetDB
// symbols into the production-visible API. See
// internal/persistence/repository/setup_test.go for the rationale on
// why Snapshot / Restore is preferred over per-test containers.
const (
	testPGImage    = "pgvector/pgvector:0.8.2-pg18-trixie"
	testDBName     = "openclarion_test"
	testDBUser     = "openclarion"
	testDBPassword = "openclarion"
)

type integrationEnv struct {
	container *postgres.PostgresContainer
	dsn       string
	client    *ent.Client
	factory   ports.UnitOfWorkFactory
}

var integration *integrationEnv

// TestMain owns the container lifecycle. A failure here (Docker
// missing on CI, image pull failure, etc.) fails the whole package
// rather than t.Skip-ing so the gate stays loud.
func TestMain(m *testing.M) {
	os.Exit(runMain(m))
}

func runMain(m *testing.M) int {
	ctx := context.Background()

	ctr, err := postgres.Run(
		ctx,
		testPGImage,
		postgres.WithDatabase(testDBName),
		postgres.WithUsername(testDBUser),
		postgres.WithPassword(testDBPassword),
		postgres.BasicWaitStrategies(),
		// pgx is the only Postgres driver the production code
		// uses; the testcontainers postgres module needs the
		// matching driver name to drive Snapshot/Restore.
		postgres.WithSQLDriver("pgx"),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "testcontainers postgres start: %v\n", err)
		return 1
	}
	defer func() {
		if terr := ctr.Terminate(ctx); terr != nil {
			fmt.Fprintf(os.Stderr, "terminate postgres container: %v\n", terr)
		}
	}()

	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "postgres connection string: %v\n", err)
		return 1
	}

	// Schema.Create runs against a throwaway client whose backend
	// connections are explicitly closed before ctr.Snapshot.
	// Snapshot uses CREATE DATABASE ... WITH TEMPLATE ... which
	// Postgres rejects (SQLSTATE 55006) when any session is still
	// attached to the template database.
	migrateDB, err := sql.Open("pgx", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open postgres for migrate: %v\n", err)
		return 1
	}
	if _, err := migrateDB.ExecContext(ctx, "CREATE EXTENSION IF NOT EXISTS vector"); err != nil {
		_ = migrateDB.Close()
		fmt.Fprintf(os.Stderr, "enable pgvector extension: %v\n", err)
		return 1
	}
	migrateDrv := entsql.OpenDB(dialect.Postgres, migrateDB)
	migrateClient := ent.NewClient(ent.Driver(migrateDrv))
	if err := migrateClient.Schema.Create(ctx); err != nil {
		_ = migrateClient.Close()
		fmt.Fprintf(os.Stderr, "create ent schema: %v\n", err)
		return 1
	}
	if err := migrateClient.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "close migrate client: %v\n", err)
		return 1
	}

	if err := ctr.Snapshot(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "snapshot postgres: %v\n", err)
		return 1
	}

	integration = &integrationEnv{
		container: ctr,
		dsn:       dsn,
	}
	if err := integration.openClient(); err != nil {
		fmt.Fprintf(os.Stderr, "open ent client: %v\n", err)
		return 1
	}
	defer func() {
		if integration.client != nil {
			_ = integration.client.Close()
		}
	}()

	return m.Run()
}

// openClient (re)builds the long-lived ent.Client + factory pair
// from integration.dsn. Called from TestMain at start-up and from
// resetDB after every snapshot restore.
func (e *integrationEnv) openClient() error {
	db, err := sql.Open("pgx", e.dsn)
	if err != nil {
		return fmt.Errorf("open postgres: %w", err)
	}
	drv := entsql.OpenDB(dialect.Postgres, db)
	e.client = ent.NewClient(ent.Driver(drv))
	e.factory = repository.NewFactory(e.client)
	return nil
}

// resetDB restores the post-migration snapshot and rebuilds the
// ent.Client + factory pair. Postgres' Restore calls
// pg_terminate_backend on every connection attached to the target
// database; the *sql.DB pool inside the old ent.Client therefore
// holds closed file descriptors that database/sql does not probe
// before reuse, so we close + reopen rather than chase stale
// connections with retry logic in production code.
func resetDB(t *testing.T) {
	t.Helper()
	if err := integration.container.Restore(context.Background()); err != nil {
		t.Fatalf("restore postgres snapshot: %v", err)
	}
	if err := integration.client.Close(); err != nil {
		t.Fatalf("close stale ent client: %v", err)
	}
	if err := integration.openClient(); err != nil {
		t.Fatalf("reopen ent client after restore: %v", err)
	}
}
