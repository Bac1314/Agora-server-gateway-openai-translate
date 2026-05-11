package main

import (
	"bytes"
	"encoding/json"
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

func TestStore_Create_Basic(t *testing.T) {
	store := newStore(10, os.Args[0])
	store.buildCmd = func(sess *Session, binary string) *exec.Cmd {
		cmd := exec.Command(binary)
		cmd.Env = append(os.Environ(), "GO_FAKE_BOT=1")
		return cmd
	}
	req := createRequest{
		AgoraAppID: "app", OpenAIKey: "key", Channel: "ch",
		SrcLang: "en", DstLang: "es", IdleExitSeconds: 300,
	}
	sess, err := store.Create(req)
	if err != nil {
		t.Fatal(err)
	}
	if sess.ID == "" {
		t.Fatal("sessionId empty")
	}
	if sess.Status != "running" {
		t.Fatalf("want running, got %s", sess.Status)
	}
	if sess.BotUID < 2000 || sess.BotUID > 2999 {
		t.Fatalf("auto-assigned uid %d out of range", sess.BotUID)
	}
	store.Stop(sess.ID)
}

func TestStore_Create_MaxSessions(t *testing.T) {
	store := newStore(2, os.Args[0])
	store.buildCmd = func(sess *Session, binary string) *exec.Cmd {
		cmd := exec.Command(binary)
		cmd.Env = append(os.Environ(), "GO_FAKE_BOT=1")
		return cmd
	}
	req := func(ch string) createRequest {
		return createRequest{AgoraAppID: "a", OpenAIKey: "k", Channel: ch, SrcLang: "en", DstLang: "es", IdleExitSeconds: 300}
	}
	if _, err := store.Create(req("ch1")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(req("ch2")); err != nil {
		t.Fatal(err)
	}
	_, err := store.Create(req("ch3"))
	if err != ErrMaxSessions {
		t.Fatalf("want ErrMaxSessions, got %v", err)
	}
	for _, s := range store.List() {
		store.Stop(s.ID)
	}
}

func TestStore_Create_Duplicate(t *testing.T) {
	store := newStore(10, os.Args[0])
	store.buildCmd = func(sess *Session, binary string) *exec.Cmd {
		cmd := exec.Command(binary)
		cmd.Env = append(os.Environ(), "GO_FAKE_BOT=1")
		return cmd
	}
	uid := 2500
	req := createRequest{AgoraAppID: "a", OpenAIKey: "k", Channel: "ch", SrcLang: "en", DstLang: "es", IdleExitSeconds: 300, BotUID: &uid}
	if _, err := store.Create(req); err != nil {
		t.Fatal(err)
	}
	_, err := store.Create(req)
	if err != ErrDuplicate {
		t.Fatalf("want ErrDuplicate, got %v", err)
	}
	for _, s := range store.List() {
		store.Stop(s.ID)
	}
}

func truePath(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("true")
	if err != nil {
		t.Skip("true not found in PATH:", err)
	}
	return p
}

func TestStore_Stop_NotFound(t *testing.T) {
	store := newStore(10, truePath(t))
	if err := store.Stop("nope"); err != ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestStore_AutoCleanup(t *testing.T) {
	tp := truePath(t)
	store := newStore(10, tp)
	store.buildCmd = func(sess *Session, binary string) *exec.Cmd {
		return exec.Command(tp)
	}
	req := createRequest{AgoraAppID: "a", OpenAIKey: "k", Channel: "ch", SrcLang: "en", DstLang: "es", IdleExitSeconds: 300}
	sess, err := store.Create(req)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		got, _ := store.Get(sess.ID)
		if got != nil && got.Status == "exited" {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("session did not transition to exited within 2s")
}
