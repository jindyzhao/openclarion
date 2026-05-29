package main

import "database/sql"

func open(dsn string) (*sql.DB, error) {
	return sql.Open("pgx", dsn)
}
