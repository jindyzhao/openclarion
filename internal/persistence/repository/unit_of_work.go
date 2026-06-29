package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	// pgx/v5/stdlib registers the "pgx" driver name with database/sql.
	// The package is imported for its init side effect; no symbols are
	// referenced directly in this file.
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/openclarion/openclarion/internal/persistence/ent"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

// openPingTimeout caps the boot-time round-trip used by
// OpenPostgres to validate the connection. 5s is generous enough to
// tolerate a cold-started Postgres on a slow CI worker but short
// enough that a misconfigured DSN does not silently delay startup.
const openPingTimeout = 5 * time.Second

// OpenPostgres opens a Postgres connection through pgx/v5/stdlib,
// pings it to verify the DSN/credentials/network actually work, and
// returns an *ent.Client. The caller MUST invoke client.Close() at
// process shutdown; that call cascades through the Ent driver and
// closes the underlying *sql.DB connection pool.
//
// dsn accepts any libpq-compatible URL or keyword/value string, e.g.
// "postgres://user:pw@host:5432/db?sslmode=disable" or
// "host=localhost port=5432 user=... dbname=... sslmode=disable".
//
// Why ping at open: database/sql's sql.Open lazily defers connection
// until the first query, so a misconfigured DSN would not surface
// until the first persistence call. We force a round-trip here so
// boot fails fast against a 5s timeout (see openPingTimeout). On
// ping failure the underlying *sql.DB is closed before returning.
//
// This helper is intentionally thin: production main wiring may need
// to set sql.DB pool sizes, statement timeouts, or instrumentation
// before constructing the Ent client. In that case callers should
// open the *sql.DB directly and pass it to NewClientFromDB.
func OpenPostgres(ctx context.Context, dsn string) (*ent.Client, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, openPingTimeout)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		// Drop the pool we just opened; the caller never sees it.
		// Close error is intentionally ignored: the meaningful
		// failure is the ping, not a teardown side-effect.
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return NewClientFromDB(db), nil
}

// NewClientFromDB constructs an Ent client backed by the provided
// *sql.DB. The caller retains ownership of the *sql.DB and is
// responsible for its lifecycle.
func NewClientFromDB(db *sql.DB) *ent.Client {
	drv := entsql.OpenDB(dialect.Postgres, db)
	return ent.NewClient(ent.Driver(drv))
}

// NewFactory wraps an Ent client as a UnitOfWorkFactory. The factory
// borrows the client's connection pool but does not take ownership;
// the caller MUST keep the client alive for the factory's lifetime
// and close it at shutdown.
func NewFactory(client *ent.Client) ports.UnitOfWorkFactory {
	return &factory{client: client}
}

// factory is the Ent-backed implementation of
// ports.UnitOfWorkFactory.
type factory struct {
	client *ent.Client
}

// Begin starts a new Postgres transaction and returns a UnitOfWork
// scoped to it. Callers MUST call exactly one of Commit / Rollback.
func (f *factory) Begin(ctx context.Context) (ports.UnitOfWork, error) {
	tx, err := f.client.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	return newUnitOfWork(tx), nil
}

// txContextKey is the unexported sentinel used to mark a context as
// "already inside an active WithinTx boundary". Being unexported
// makes the marker unforgeable from outside this package: only
// factory.WithinTx (below) can set it, and only factory.WithinTx
// observes it. We store struct{}{} as the value because a non-nil
// presence is all that matters; the implementation detects nesting
// via ctx.Value(txContextKey{}) != nil rather than by value
// identity, which keeps the marker cheap (zero allocation) and
// makes future ports.UnitOfWorkFactory implementations free to
// adopt the same shape without coordinating on a value.
type txContextKey struct{}

// WithinTx runs fn inside a Begin / Commit / Rollback boundary.
//
// Semantics:
//   - if fn returns nil, the transaction is committed and the commit
//     error (if any) is returned;
//   - if fn returns a non-nil error, the transaction is rolled back
//     and fn's error is returned (rollback errors are joined);
//   - if fn panics, the transaction is rolled back and the panic is
//     re-raised after rollback completes.
//
// Nesting protection: WithinTx checks ctx for the txContextKey
// sentinel and returns ports.ErrNestedTransaction (without opening a
// new transaction) when the caller is already inside a WithinTx
// boundary. The ctx passed to fn carries the marker, so any deeper
// helper that calls WithinTx with the same ctx (or a ctx derived
// from it) is short-circuited the same way. This protects the UoW
// atomicity guarantee against composition bugs where an inner
// commit could outlive an outer rollback. Callers that genuinely
// want nested-tx semantics should design around savepoints, which
// are not exposed in this PR.
func (f *factory) WithinTx(ctx context.Context, fn func(context.Context, ports.UnitOfWork) error) error {
	if ctx.Value(txContextKey{}) != nil {
		return ports.ErrNestedTransaction
	}
	tx, err := f.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	uow := newUnitOfWork(tx)
	innerCtx := context.WithValue(ctx, txContextKey{}, struct{}{})
	defer func() {
		if v := recover(); v != nil {
			// Best-effort rollback on panic. We deliberately drop
			// any rollback error: re-panicking with the original
			// payload preserves the program's existing crash
			// semantics and is more important than reporting the
			// rollback path.
			_ = uow.rollbackInternal()
			panic(v)
		}
	}()
	if err := fn(innerCtx, uow); err != nil {
		if rerr := uow.rollbackInternal(); rerr != nil {
			return fmt.Errorf("%w (additionally, rollback failed: %w)", err, rerr)
		}
		return err
	}
	if cerr := uow.commitInternal(); cerr != nil {
		return fmt.Errorf("commit transaction: %w", cerr)
	}
	return nil
}

// closedState tracks whether a UoW has been committed or rolled back.
// We use atomic.Int32 so a stale Commit / Rollback after WithinTx
// returns deterministic errors instead of double-closing the
// underlying *ent.Tx (which Ent does not allow).
type closedState int32

const (
	uowOpen closedState = iota
	uowCommitted
	uowRolledBack
)

// unitOfWork is the Ent-backed implementation of ports.UnitOfWork.
// Repository instances are constructed lazily on first access so a
// transaction that uses only one aggregate-root does not pay for the
// others.
type unitOfWork struct {
	tx     *ent.Tx
	closed atomic.Int32

	alerts    *alertRepo
	evidence  *evidenceRepo
	diagnosis *diagnosisRepo
	reports   *reportRepo
	config    *configRepo
	directory *directoryRepo
	rbac      *rbacRepo
}

func newUnitOfWork(tx *ent.Tx) *unitOfWork {
	uow := &unitOfWork{tx: tx}
	uow.alerts = &alertRepo{tx: tx, closed: &uow.closed}
	uow.evidence = &evidenceRepo{tx: tx, closed: &uow.closed}
	uow.diagnosis = &diagnosisRepo{tx: tx, closed: &uow.closed}
	uow.reports = &reportRepo{tx: tx, closed: &uow.closed}
	uow.config = &configRepo{tx: tx, closed: &uow.closed}
	uow.directory = &directoryRepo{tx: tx, closed: &uow.closed}
	uow.rbac = &rbacRepo{tx: tx, closed: &uow.closed}
	return uow
}

// errUoWClosed is returned by every method on a UoW (or one of its
// repositories) after Commit or Rollback has been called.
var errUoWClosed = errors.New("repository: unit of work is closed")

func (u *unitOfWork) Alerts() ports.AlertRepository         { return u.alerts }
func (u *unitOfWork) Evidence() ports.EvidenceRepository    { return u.evidence }
func (u *unitOfWork) Diagnosis() ports.DiagnosisRepository  { return u.diagnosis }
func (u *unitOfWork) Reports() ports.ReportRepository       { return u.reports }
func (u *unitOfWork) Config() ports.ConfigurationRepository { return u.config }
func (u *unitOfWork) Directory() ports.DirectoryRepository  { return u.directory }
func (u *unitOfWork) RBAC() ports.RBACRepository            { return u.rbac }

// Commit finalises the transaction. After a successful Commit the
// UoW is closed; subsequent Commit / Rollback calls return
// errUoWClosed.
func (u *unitOfWork) Commit(_ context.Context) error {
	return u.commitInternal()
}

// Rollback aborts the transaction. After Rollback the UoW is closed.
func (u *unitOfWork) Rollback(_ context.Context) error {
	return u.rollbackInternal()
}

func (u *unitOfWork) commitInternal() error {
	if !u.closed.CompareAndSwap(int32(uowOpen), int32(uowCommitted)) {
		return errUoWClosed
	}
	if err := u.tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

func (u *unitOfWork) rollbackInternal() error {
	if !u.closed.CompareAndSwap(int32(uowOpen), int32(uowRolledBack)) {
		return errUoWClosed
	}
	if err := u.tx.Rollback(); err != nil {
		return fmt.Errorf("rollback transaction: %w", err)
	}
	return nil
}

// checkOpen returns errUoWClosed when the parent UoW has been
// committed or rolled back. Repository methods invoke it as the very
// first action so callers see a consistent error rather than the
// driver-level "transaction has already been committed or rolled
// back" message.
func checkOpen(closed *atomic.Int32) error {
	if closed.Load() != int32(uowOpen) {
		return errUoWClosed
	}
	return nil
}
