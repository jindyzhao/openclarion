package repository

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/persistence/ent"
)

// pgErrUniqueViolation is the Postgres SQLSTATE for a unique
// constraint violation. The literal "23505" is documented in the
// Postgres manual (Appendix A: Error Codes); we keep a named
// constant so grep-ability is preserved across the codebase.
const pgErrUniqueViolation = "23505"

// asAlreadyExists translates Postgres unique-violation errors
// (SQLSTATE 23505) raised through Ent into a wrapped
// domain.ErrAlreadyExists. Other constraint errors (foreign-key,
// check, not-null) propagate verbatim because the usecase layer
// MUST treat them as bugs, not idempotency boundaries.
//
// Returns nil when err is nil. Returns the original err unchanged
// when it does not embed a *pgconn.PgError with code 23505.
func asAlreadyExists(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgErrUniqueViolation {
		// pgErr.ConstraintName is the index name in Postgres; we
		// surface it for debugging but the wrapped sentinel is
		// what callers branch on.
		return fmt.Errorf("%w: constraint %q violated", domain.ErrAlreadyExists, pgErr.ConstraintName)
	}
	return err
}

// asNotFound translates Ent's NotFoundError into a wrapped
// domain.ErrNotFound. Other errors propagate verbatim.
//
// Returns nil when err is nil.
func asNotFound(err error) error {
	if err == nil {
		return nil
	}
	if ent.IsNotFound(err) {
		return fmt.Errorf("%w", domain.ErrNotFound)
	}
	return err
}
