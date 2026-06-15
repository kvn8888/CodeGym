package response

import (
	"encoding/json"
	"net/http"
)

type envelope struct {
	Data  any       `json:"data"`
	Error *apiError `json:"error"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func JSON(w http.ResponseWriter, status int, data any) {
	write(w, status, envelope{Data: data, Error: nil})
}

func NoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

func Error(w http.ResponseWriter, status int, code, message string) {
	write(w, status, envelope{
		Data: nil,
		Error: &apiError{
			Code:    code,
			Message: message,
		},
	})
}

func write(w http.ResponseWriter, status int, body envelope) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
