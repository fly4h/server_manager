package api

import (
	"database/sql"
	"runtime"
	"strconv"
)

// aliases keep handler code readable
type sqlNullString = sql.NullString
type sqlNullInt64 = sql.NullInt64

func sqlNullInt64Of(i int64) sql.NullInt64 { return sql.NullInt64{Int64: i, Valid: true} }

func nullInt64(i int64) any {
	if i == 0 {
		return nil
	}
	return i
}

func strconvI64(i int64) string { return strconv.FormatInt(i, 10) }

var goVersion = runtime.Version()
