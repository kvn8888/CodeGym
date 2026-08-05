package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kvn8888/codegym/backend/internal/environment"
)

func TestReadyPayloadIncludesEnvironment(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
		mode string
	}{
		{name: "memory", mode: "memory"},
		{name: "postgres", dsn: "postgres://example", mode: "postgres"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := NewReadyHandlerWithPinger(test.dsn, environment.Stg, func(context.Context, string) error {
				return nil
			})
			response := httptest.NewRecorder()
			handler(response, httptest.NewRequest(http.MethodGet, "/ready", nil))

			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", response.Code)
			}
			var envelope struct {
				Data map[string]string `json:"data"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("decode payload: %v", err)
			}
			payload := envelope.Data
			if payload["status"] != "ok" || payload["mode"] != test.mode || payload["environment"] != "stg" {
				t.Fatalf("payload = %#v", payload)
			}
		})
	}
}

func TestReadyFailureDoesNotExposeDatabaseDetails(t *testing.T) {
	handler := NewReadyHandlerWithPinger("postgres://secret", environment.Prd, func(context.Context, string) error {
		return errors.New("driver detail")
	})
	response := httptest.NewRecorder()
	handler(response, httptest.NewRequest(http.MethodGet, "/ready", nil))

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", response.Code)
	}
	if body := response.Body.String(); body == "" || strings.Contains(body, "postgres://secret") || strings.Contains(body, "driver detail") {
		t.Fatalf("unsafe readiness body = %q", body)
	}
}
