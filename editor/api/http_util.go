package api

import (
	"encoding/json"
	"log"
	"net/http"
)

// writeJSON serializes v as JSON with the given status code. Errors are
// logged but swallowed — at this point the response has been committed.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("editor: writeJSON: %v", err)
	}
}

// writeErr writes a uniform error body: {"error": msg}.
func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
