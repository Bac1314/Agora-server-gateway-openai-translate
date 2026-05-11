package main

import (
	"net/http"
)

func handleCreateSession(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not implemented", http.StatusNotImplemented)
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
