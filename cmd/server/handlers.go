package main

import (
	"encoding/json"
	"errors"
	"net/http"
)

func handleCreateSession(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if req.AgoraAppID == "" || req.OpenAIKey == "" || req.Channel == "" {
			writeError(w, http.StatusBadRequest, "agoraAppId, openAiKey, and channel are required")
			return
		}
		if req.SrcLang == "" {
			req.SrcLang = "en"
		}
		if req.DstLang == "" {
			req.DstLang = "es"
		}
		if req.IdleExitSeconds == 0 {
			req.IdleExitSeconds = 300
		}
		sess, err := store.Create(req)
		if err != nil {
			switch {
			case errors.Is(err, ErrMaxSessions), errors.Is(err, ErrUIDPoolExhausted):
				writeError(w, http.StatusServiceUnavailable, err.Error())
			case errors.Is(err, ErrDuplicate):
				writeError(w, http.StatusConflict, err.Error())
			default:
				writeError(w, http.StatusInternalServerError, "failed to start session")
			}
			return
		}
		snapshot := *sess
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(&snapshot)
	}
}

func handleListSessions(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not implemented", http.StatusNotImplemented)
	}
}

func handleGetSession(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not implemented", http.StatusNotImplemented)
	}
}

func handleDeleteSession(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not implemented", http.StatusNotImplemented)
	}
}
