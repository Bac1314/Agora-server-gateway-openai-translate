package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
)

func main() {
	apiKey := os.Getenv("API_KEY")
	if apiKey == "" {
		log.Fatal("API_KEY env var is required")
	}
	botBinary := envString("BOT_BINARY", "/app/agora_rtc_sdk/example/out/translator_bot")
	maxSessions := envInt("MAX_SESSIONS", 10)
	port := envString("PORT", "8080")

	store := newStore(maxSessions, botBinary)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handleHealth(store))
	mux.HandleFunc("POST /sessions", handleCreateSession(store))
	mux.HandleFunc("GET /sessions", handleListSessions(store))
	mux.HandleFunc("GET /sessions/{id}", handleGetSession(store))
	mux.HandleFunc("DELETE /sessions/{id}", handleDeleteSession(store))

	log.Printf("starting on :%s  max-sessions=%d  bot=%s", port, maxSessions, botBinary)
	log.Fatal(http.ListenAndServe(":"+port, authMiddleware(apiKey, mux)))
}

func authMiddleware(apiKey string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != apiKey {
			writeError(w, http.StatusUnauthorized, "invalid or missing API key")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func handleHealth(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status":   "ok",
			"sessions": store.ActiveCount(),
		})
	}
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func envString(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			log.Fatalf("%s must be a positive integer, got %q", key, v)
		}
		return n
	}
	return def
}
