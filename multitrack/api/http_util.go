package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"time"
)

// writeJSON serializes v as JSON with the given status code. Errors are
// logged but swallowed — at this point the response has been committed.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("multitrack: writeJSON: %v", err)
	}
}

// writeErr writes a uniform error body: {"error": msg}. Same shape as
// editor/api so the frontend client (api/client.ts) can treat both
// modules identically.
func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// newID returns a random 8-hex id for a project. Same generator the
// single-video editor uses; fallback path is here so a crypto/rand
// failure never blocks a project create.
func newID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "id" + time.Now().UTC().Format("150405")
	}
	return hex.EncodeToString(b[:])
}
