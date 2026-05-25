package repository

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/openclarion/openclarion/internal/persistence/ent"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

// Test fixtures use a single Postgres container shared across the
// whole package. Each test that touches the database calls
// resetDB(t) at the top of its body to restore the post-migration
// snapshot, so individual tests see an empty schema regardless of
// execution order. Snapshot/Restore is a few hundred ms per call;
// running every test in its own container would dominate wall clock.
//
// Image / credentials are package-private constants so the cost of
// upgrading Postgres is one diff, not a sprawl of literals.
const (
	testPGImage    = "postgres:18-alpine"
	testDBName     = "openclarion_test"
	testDBUser     = "openclarion"
	testDBPassword = "openclarion"
)

// integrationEnv carries the live container + Ent client + factory
// for the whole test binary. Populated by TestMain; nil when the
// test binary is built but TestMain has not yet been entered.
//
// dsn is retained so resetDB can rebuild the *sql.DB pool from
// scratch after each Restore (see resetDB for why a rebuild is
// required rather than a simple reuse).
type integrationEnv struct {
	container *postgres.PostgresContainer
	dsn       string
	client    *ent.Client
	factory   ports.UnitOfWorkFactory
}

var integration *integrationEnv

// TestMain owns the container lifecycle. It is the only place that
// is allowed to call postgres.Run / Schema.Create / Snapshot. All
// other test files MUST go through resetDB + integration.factory.
//
// Errors here mean the container could not start (Docker missing on
// the runner, image pull failure, etc.). We deliberately fail the
// whole package rather than t.Skip, so missing Docker on CI shows up
// as a red gate instead of a silently-passing one.
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
		// same driver name to drive Snapshot/Restore.
		postgres.WithSQLDriver("pgx"),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "testcontainers postgres start: %v\n", err)
		return 1
	}
	defer func() {
		// Best-effort terminate: returning a non-zero exit code
		// for a teardown failure would mask real test failures.
		if terr := ctr.Terminate(ctx); terr != nil {
			fmt.Fprintf(os.Stderr, "terminate postgres container: %v\n", terr)
		}
	}()

	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "postgres connection string: %v\n", err)
		return 1
	}

	// Schema.Create produces the same DDL as the committed Atlas
	// migrations because both derive from the Ent schema package
	// at internal/persistence/ent/schema. We run it in a *throwaway*
	// client so we can close every Postgres backend it spawned
	// before calling ctr.Snapshot. Snapshot performs
	//   CREATE DATABASE migrated_template WITH TEMPLATE openclarion_test
	// which Postgres rejects (SQLSTATE 55006) if any session is still
	// connected to the template database.
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

	// Snapshot the empty post-migration database. resetDB restores
	// to this point.
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
	e.factory = NewFactory(e.client)
	return nil
}

// resetDB restores the post-migration snapshot and rebuilds the
// ent.Client + factory pair so the next test sees a clean schema
// against a fresh connection pool.
//
// Postgres' Restore implementation runs pg_terminate_backend against
// every connection on the target database; that leaves the *sql.DB
// pool inside ent.Client holding closed file descriptors. database/sql
// does *not* probe idle connections before handing them out, so the
// next BeginTx on the old client surfaces the terminated connection
// as SQLSTATE 57P01. Closing the client + reopening sidesteps this
// without leaking pgx-specific retry logic into production code.
//
// Tests that touch the database MUST call this in the very first
// line so the test sees a clean schema regardless of execution
// order.
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
