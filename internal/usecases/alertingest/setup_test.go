package alertingest_test

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

// The alertingest package needs an end-to-end Postgres for its
// integration tests so the ErrAlreadyExists-on-duplicate path
// (which only manifests once a real unique constraint fires) is
// covered. We deliberately re-create the testcontainers harness
// inside this package rather than promoting repository/setup_test.go
// to a shared helper: cross-package fixtures would force the
// internal `integration` / `resetDB` symbols to leak into the
// production-visible API, and the duplicated boilerplate here is a
// few dozen lines.
//
// Image / credential constants mirror repository/setup_test.go so
// upgrading Postgres stays a localised diff per package.
const (
	testPGImage    = "postgres:18-alpine"
	testDBName     = "openclarion_test"
	testDBUser     = "openclarion"
	testDBPassword = "openclarion"
)

// integrationEnv carries the live container + Ent client + factory
// for the whole alertingest test binary. dsn is retained so resetDB
// can rebuild the *sql.DB pool from scratch after every snapshot
// restore (see resetDB for why a rebuild is required).
type integrationEnv struct {
	container *postgres.PostgresContainer
	dsn       string
	client    *ent.Client
	factory   ports.UnitOfWorkFactory
}

var integration *integrationEnv

// TestMain owns the container lifecycle. Failure here (Docker
// missing, image pull failure, etc.) fails the whole package so a
// broken integration runner shows as a red gate rather than as a
// silent green skip.
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
	// connections are explicitly closed BEFORE ctr.Snapshot.
	// Snapshot uses
	//   CREATE DATABASE migrated_template WITH TEMPLATE openclarion_test
	// which Postgres rejects (SQLSTATE 55006) when any session is
	// still attached to the template database.
	migrateDB, err := sql.Open("pgx", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open postgres for migrate: %v\n", err)
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
// ent.Client + factory pair so the next test sees a clean schema
// against a fresh connection pool.
//
// Postgres' Restore runs pg_terminate_backend on every connection
// attached to the target database; this leaves the *sql.DB pool
// inside ent.Client holding closed file descriptors. database/sql
// does not probe idle connections before handing them out, so the
// next BeginTx on the stale client surfaces the terminated
// connection as SQLSTATE 57P01. Closing + reopening sidesteps that
// without leaking pgx-specific retry logic into production code.
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
