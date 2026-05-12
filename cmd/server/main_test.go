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
			store.Stop(sess.ID) //nolint:errcheck
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
	t.Cleanup(func() {
		for _, s := range store.List() {
			store.Stop(s.ID) //nolint:errcheck
		}
		time.Sleep(100 * time.Millisecond)
	})
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
}

func TestStore_Create_MaxSessions(t *testing.T) {
	store := newStore(2, os.Args[0])
	store.buildCmd = func(sess *Session, binary string) *exec.Cmd {
		cmd := exec.Command(binary)
		cmd.Env = append(os.Environ(), "GO_FAKE_BOT=1")
		return cmd
	}
	t.Cleanup(func() {
		for _, s := range store.List() {
			store.Stop(s.ID) //nolint:errcheck
		}
		time.Sleep(100 * time.Millisecond)
	})
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
}

func TestStore_Create_Duplicate(t *testing.T) {
	store := newStore(10, os.Args[0])
	store.buildCmd = func(sess *Session, binary string) *exec.Cmd {
		cmd := exec.Command(binary)
		cmd.Env = append(os.Environ(), "GO_FAKE_BOT=1")
		return cmd
	}
	t.Cleanup(func() {
		for _, s := range store.List() {
			store.Stop(s.ID) //nolint:errcheck
		}
		time.Sleep(100 * time.Millisecond)
	})
	uid := 2500
	req := createRequest{AgoraAppID: "a", OpenAIKey: "k", Channel: "ch", SrcLang: "en", DstLang: "es", IdleExitSeconds: 300, BotUID: &uid}
	if _, err := store.Create(req); err != nil {
		t.Fatal(err)
	}
	_, err := store.Create(req)
	if err != ErrDuplicate {
		t.Fatalf("want ErrDuplicate, got %v", err)
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
		got, ok := store.Get(sess.ID)
		if ok && got.Status == "exited" {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("session did not transition to exited within 2s")
}

func TestHealth(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := do(t, srv, "GET", "/health", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Fatalf("want status=ok, got %v", body["status"])
	}
}

func TestAuthRequired(t *testing.T) {
	srv, _ := newTestServer(t)
	req, _ := http.NewRequest("GET", srv.URL+"/health", nil)
	// deliberately no X-Api-Key header
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 401 {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestCreateSession(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := do(t, srv, "POST", "/sessions", validBody())
	if resp.StatusCode != 201 {
		t.Fatalf("want 201, got %d", resp.StatusCode)
	}
	var sess Session
	json.NewDecoder(resp.Body).Decode(&sess)
	if sess.ID == "" {
		t.Fatal("sessionId is empty")
	}
	if sess.Channel != "test-channel" {
		t.Fatalf("want channel=test-channel, got %s", sess.Channel)
	}
	if sess.Status != "running" {
		t.Fatalf("want status=running, got %s", sess.Status)
	}
	if sess.SrcLang != "en" {
		t.Fatalf("want srcLang=en, got %s", sess.SrcLang)
	}
}

func TestCreateSession_MissingFields(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := do(t, srv, "POST", "/sessions", map[string]any{"channel": "x"})
	if resp.StatusCode != 400 {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}

func TestCreateSession_MaxSessions(t *testing.T) {
	srv, _ := newTestServer(t) // max=3 in newTestServer
	for i := range 3 {
		body := validBody()
		body["channel"] = fmt.Sprintf("ch-%d", i)
		resp := do(t, srv, "POST", "/sessions", body)
		if resp.StatusCode != 201 {
			t.Fatalf("session %d: want 201, got %d", i, resp.StatusCode)
		}
	}
	resp := do(t, srv, "POST", "/sessions", map[string]any{
		"agoraAppId": "id", "openAiKey": "key", "channel": "overflow",
	})
	if resp.StatusCode != 503 {
		t.Fatalf("want 503, got %d", resp.StatusCode)
	}
}

func TestCreateSession_Duplicate(t *testing.T) {
	srv, _ := newTestServer(t)
	uid := 2500
	body := validBody()
	body["botUid"] = uid
	if do(t, srv, "POST", "/sessions", body).StatusCode != 201 {
		t.Fatal("first create failed")
	}
	resp := do(t, srv, "POST", "/sessions", body)
	if resp.StatusCode != 409 {
		t.Fatalf("want 409, got %d", resp.StatusCode)
	}
}

func TestCreateSession_AutoUID(t *testing.T) {
	srv, _ := newTestServer(t)
	var sess Session
	json.NewDecoder(do(t, srv, "POST", "/sessions", validBody()).Body).Decode(&sess)
	if sess.BotUID < 2000 || sess.BotUID > 2999 {
		t.Fatalf("auto-assigned botUid %d out of [2000,2999]", sess.BotUID)
	}
}

func TestListSessions(t *testing.T) {
	srv, _ := newTestServer(t)
	do(t, srv, "POST", "/sessions", validBody())
	resp := do(t, srv, "GET", "/sessions", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var sessions []Session
	json.NewDecoder(resp.Body).Decode(&sessions)
	if len(sessions) != 1 {
		t.Fatalf("want 1 session, got %d", len(sessions))
	}
}

func TestGetSession(t *testing.T) {
	srv, _ := newTestServer(t)
	var created Session
	json.NewDecoder(do(t, srv, "POST", "/sessions", validBody()).Body).Decode(&created)

	resp := do(t, srv, "GET", "/sessions/"+created.ID, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var sess Session
	json.NewDecoder(resp.Body).Decode(&sess)
	if sess.ID != created.ID {
		t.Fatalf("want id=%s, got %s", created.ID, sess.ID)
	}
	if sess.ExitCode != nil {
		t.Fatalf("exitCode should be nil while running, got %v", sess.ExitCode)
	}
}

func TestGetSession_NotFound(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := do(t, srv, "GET", "/sessions/doesnotexist", nil)
	if resp.StatusCode != 404 {
		t.Fatalf("want 404, got %d", resp.StatusCode)
	}
}

func TestDeleteSession(t *testing.T) {
	srv, _ := newTestServer(t)
	var created Session
	json.NewDecoder(do(t, srv, "POST", "/sessions", validBody()).Body).Decode(&created)

	resp := do(t, srv, "DELETE", "/sessions/"+created.ID, nil)
	if resp.StatusCode != 204 {
		t.Fatalf("want 204, got %d", resp.StatusCode)
	}
}

func TestDeleteSession_NotFound(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := do(t, srv, "DELETE", "/sessions/doesnotexist", nil)
	if resp.StatusCode != 404 {
		t.Fatalf("want 404, got %d", resp.StatusCode)
	}
}

func TestDeleteSession_AlreadyExited(t *testing.T) {
	_, store := newTestServer(t)
	store.buildCmd = func(sess *Session, binary string) *exec.Cmd {
		return exec.Command(truePath(t))
	}
	req := createRequest{
		AgoraAppID: "a", OpenAIKey: "k", Channel: "ch",
		SrcLang: "en", DstLang: "es", IdleExitSeconds: 300,
	}
	sess, err := store.Create(req)
	if err != nil {
		t.Fatal(err)
	}
	// wait for process to exit and session to be marked exited
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		got, ok := store.Get(sess.ID)
		if ok && got.Status == "exited" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	// Stop on exited session should return ErrNotFound
	if err := store.Stop(sess.ID); err != ErrNotFound {
		t.Fatalf("want ErrNotFound on exited session, got %v", err)
	}
}
