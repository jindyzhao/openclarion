package baddbopen

import "database/sql"

func open(dsn string) (*sql.DB, error) {
	return sql.Open("pgx", dsn) // want "production code must not call database/sql.Open outside the persistence repository boundary"
}
