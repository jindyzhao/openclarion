package gooddbtest

import "database/sql"

func openForTest(dsn string) (*sql.DB, error) {
	return sql.Open("pgx", dsn)
}
