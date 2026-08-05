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
	"github.com/kvn8888/codegym/backend/internal/environment"
)

type readinessPinger func(context.Context, string) error

// pingPostgres opens a short-lived database/sql handle and verifies
// connectivity within a bounded timeout.
var pingPostgres readinessPinger = func(ctx context.Context, dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer func() {
		_ = db.Close()
	}()

	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	return db.PingContext(pingCtx)
}

// NewReadyHandler returns an HTTP readiness handler that uses Postgres ping when a DB DSN is configured.
func NewReadyHandler(databaseDSN string, codegymEnvironment environment.Name) http.HandlerFunc {
	return NewReadyHandlerWithPinger(databaseDSN, codegymEnvironment, pingPostgres)
}

// NewReadyHandlerWithPinger builds the readiness handler with an injectable pinger for testability.
func NewReadyHandlerWithPinger(databaseDSN string, codegymEnvironment environment.Name, pinger readinessPinger) http.HandlerFunc {
	if pinger == nil {
		pinger = pingPostgres
	}

	return func(w http.ResponseWriter, r *http.Request) {
		dsn := strings.TrimSpace(databaseDSN)
		if dsn == "" {
			// In-memory mode has no external dependency to verify.
			response.JSON(w, http.StatusOK, map[string]string{
				"status":      "ok",
				"mode":        "memory",
				"environment": string(codegymEnvironment),
			})
			return
		}

		if err := pinger(r.Context(), dsn); err != nil {
			// Return a generic failure to avoid leaking DSN/driver details.
			response.Error(w, http.StatusServiceUnavailable, "not_ready", "Service unavailable.")
			return
		}

		// Postgres is configured and reachable.
		response.JSON(w, http.StatusOK, map[string]string{
			"status":      "ok",
			"mode":        "postgres",
			"environment": string(codegymEnvironment),
		})
	}
}
