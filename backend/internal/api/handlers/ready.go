package handlers

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"time"

	// run this package's init() code, but don't use any exported symbols.
	// jackc/pgx is the most popular, modern Postgres driver for Go

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/kvn8888/codegym/backend/internal/api/response"
)

type readinessPinger func(context.Context, string) error

var pingPostgres readinessPinger = func(ctx context.Context, dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	return db.PingContext(pingCtx)
}

func NewReadyHandler(databaseDSN string) http.HandlerFunc {
	return NewReadyHandlerWithPinger(databaseDSN, pingPostgres)
}

func NewReadyHandlerWithPinger(databaseDSN string, pinger readinessPinger) http.HandlerFunc {
	if pinger == nil {
		pinger = pingPostgres
	}

	return func(w http.ResponseWriter, r *http.Request) {
		dsn := strings.TrimSpace(databaseDSN)
		if dsn == "" {
			response.JSON(w, http.StatusOK, map[string]string{
				"status": "ok",
				"mode":   "memory",
			})
			return
		}

		if err := pinger(r.Context(), dsn); err != nil {
			response.Error(w, http.StatusServiceUnavailable, "not_ready", "Service unavailable.")
			return
		}

		response.JSON(w, http.StatusOK, map[string]string{
			"status": "ok",
			"mode":   "postgres",
		})
	}
}
