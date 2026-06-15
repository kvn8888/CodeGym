package handlers

import (
	"net/http"

	"github.com/kvn8888/codegym/backend/internal/api/response"
)

func Health(w http.ResponseWriter, _ *http.Request) {
	response.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
