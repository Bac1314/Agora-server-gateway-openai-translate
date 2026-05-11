package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"
	"time"
)

// When spawned as subprocess by tests, block until killed.
func TestMain(m *testing.M) {
	if os.Getenv("GO_FAKE_BOT") == "1" {
		select {}
	}
	os.Exit(m.Run())
}

func newTestServer(t *testing.T) (*httptest.Server, *Store) {
	t.Helper()
	store := newStore(3, os.Args[0])
	store.buildCmd = func(sess *Session, binary string) *exec.Cmd {
		cmd := exec.Command(binary)
		cmd.Env = append(os.Environ(), "GO_FAKE_BOT=1")
		return cmd
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handleHealth(store))
	mux.HandleFunc("POST /sessions", handleCreateSession(store))
	mux.HandleFunc("GET /sessions", handleListSessions(store))
	mux.HandleFunc("GET /sessions/{id}", handleGetSession(store))
	mux.HandleFunc("DELETE /sessions/{id}", handleDeleteSession(store))
	srv := httptest.NewServer(authMiddleware("test-key", mux))
	t.Cleanup(func() {
		for _, sess := range store.List() {
			store.Stop(sess.ID) //nolint
		}
		srv.Close()
	})
	return srv, store
}

func do(t *testing.T, srv *httptest.Server, method, path string, body any) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req, err := http.NewRequest(method, srv.URL+path, &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Api-Key", "test-key")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func validBody() map[string]any {
	return map[string]any{
		"agoraAppId": "test-app-id",
		"openAiKey":  "sk-test",
		"channel":    "test-channel",
	}
}

// suppress unused import warning until tests are added
var _ = fmt.Sprintf
var _ = time.Second
