// Package dberr classifies PostgreSQL errors so repositories can translate them into domain errors.
package dberr

import (
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	uniqueViolation     = "23505"
	foreignKeyViolation = "23503"
)

func IsUnique(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == uniqueViolation
}

func IsForeignKey(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == foreignKeyViolation
}

func IsNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
