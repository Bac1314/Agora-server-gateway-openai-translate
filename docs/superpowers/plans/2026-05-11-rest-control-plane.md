# REST Control Plane Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Go HTTP server that dynamically starts/stops translator_bot subprocesses via a REST API, replacing the single-channel hardcoded daemon entrypoint.

**Architecture:** Go server (stdlib only) owns an in-memory session map protected by a mutex. Each POST /sessions spawns a translator_bot subprocess; a per-session goroutine blocks on cmd.Wait() and marks the session exited on process death. The Go binary and translator_bot binary coexist in the same Docker image; server is the new ENTRYPOINT.

**Tech Stack:** Go 1.22 stdlib (net/http, os/exec, sync, crypto/rand), Docker multi-stage build (golang:1.22 + ubuntu:22.04)

---

### Task 1: Go module + project skeleton

**Files:**
- Create: `go.mod`
- Create: `cmd/server/main.go`
- Create: `cmd/server/store.go`
- Create: `cmd/server/handlers.go`
- Create: `cmd/server/main_test.go`

- [ ] **Step 1: Create go.mod**

```
module github.com/Bac1314/Agora-server-gateway-openai-translate

go 1.22
```

- [ ] **Step 2: Create cmd/server/store.go with types only**

```go
package main

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

var (
	ErrMaxSessions      = errors.New("max sessions reached")
	ErrUIDPoolExhausted = errors.New("bot UID pool exhausted")
	ErrDuplicate        = errors.New("session already running for this channel and bot UID")
	ErrNotFound         = errors.New("session not found")
)

type Session struct {
	ID        string    `json:"sessionId"`
	BotUID    int       `json:"botUid"`
	Channel   string    `json:"channel"`
	SrcLang   string    `json:"srcLang"`
	DstLang   string    `json:"dstLang"`
	StartedAt time.Time `json:"startedAt"`
	Status    string    `json:"status"`
	ExitCode  *int      `json:"exitCode"`
	// unexported — excluded from JSON serialization
	agoraAppID      string
	openAIKey       string
	speakerUID      int
	idleExitSeconds int
	cmd             *exec.Cmd
}

type createRequest struct {
	AgoraAppID      string `json:"agoraAppId"`
	OpenAIKey       string `json:"openAiKey"`
	Channel         string `json:"channel"`
	SrcLang         string `json:"srcLang"`
	DstLang         string `json:"dstLang"`
	SpeakerUID      int    `json:"speakerUid"`
	BotUID          *int   `json:"botUid"`
	IdleExitSeconds int    `json:"idleExitSeconds"`
}

type Store struct {
	mu          sync.Mutex
	sessions    map[string]*Session
	maxSessions int
	botBinary   string
	buildCmd    func(sess *Session, binary string) *exec.Cmd
}

func newStore(maxSessions int, botBinary string) *Store {
	return &Store{
		sessions:    make(map[string]*Session),
		maxSessions: maxSessions,
		botBinary:   botBinary,
		buildCmd:    defaultBuildCmd,
	}
}

func defaultBuildCmd(sess *Session, binary string) *exec.Cmd {
	return exec.Command(binary,
		"--token", sess.agoraAppID,
		"--channelId", sess.Channel,
		"--speakerUid", fmt.Sprintf("%d", sess.speakerUID),
		"--botUid", fmt.Sprintf("%d", sess.BotUID),
		"--srcLang", sess.SrcLang,
		"--dstLang", sess.DstLang,
		"--idleExitSeconds", fmt.Sprintf("%d", sess.idleExitSeconds),
	)
}

func (s *Store) ActiveCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runningCount()
}

func (s *Store) runningCount() int {
	n := 0
	for _, sess := range s.sessions {
		if sess.Status == "running" {
			n++
		}
	}
	return n
}

// placeholder — implemented in Task 2
func (s *Store) Create(req createRequest) (*Session, error) {
	return nil, errors.New("not implemented")
}

func (s *Store) Stop(id string) error {
	return ErrNotFound
}

func (s *Store) Get(id string) (*Session, bool) {
	return nil, false
}

func (s *Store) List() []*Session {
	return nil
}

func (s *Store) resolveUID(req createRequest) (int, error) {
	return 0, errors.New("not implemented")
}

func (s *Store) watchProcess(id string, cmd *exec.Cmd) {}

func randomID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return fmt.Sprintf("%x", b)
}

// keep compiler happy — used in Task 2
var _ = syscall.SIGTERM
var _ = os.Environ
```

- [ ] **Step 3: Create cmd/server/handlers.go with stubs**

```go
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
```

- [ ] **Step 4: Create cmd/server/main.go**

```go
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
```

- [ ] **Step 5: Create cmd/server/main_test.go with TestMain + test helpers**

```go
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
```

- [ ] **Step 6: Verify it compiles**

```bash
cd /path/to/repo && go build ./cmd/server/
```

Expected: no errors (stubs satisfy compiler).

- [ ] **Step 7: Commit skeleton**

```bash
git add go.mod cmd/server/
git commit -m "feat: add Go server skeleton (stubs, types, test infrastructure)"
```

---

### Task 2: Session store — unit tests + full implementation

**Files:**
- Modify: `cmd/server/store.go` (replace placeholder methods with real implementations)
- Modify: `cmd/server/main_test.go` (add store unit tests)

- [ ] **Step 1: Add store unit tests to main_test.go**

Append these tests to `cmd/server/main_test.go`:

```go
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

func TestStore_Stop_NotFound(t *testing.T) {
	store := newStore(10, "/bin/true")
	if err := store.Stop("nope"); err != ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestStore_AutoCleanup(t *testing.T) {
	store := newStore(10, "/bin/true")
	store.buildCmd = func(sess *Session, binary string) *exec.Cmd {
		return exec.Command("/bin/true")
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
```

- [ ] **Step 2: Run tests — expect failures**

```bash
go test ./cmd/server/ -run TestStore -v
```

Expected: FAIL — `Create` returns "not implemented".

- [ ] **Step 3: Replace placeholder methods in store.go**

Replace the four placeholder methods and `watchProcess`/`resolveUID` in `cmd/server/store.go` with:

```go
func (s *Store) Create(req createRequest) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.runningCount() >= s.maxSessions {
		return nil, ErrMaxSessions
	}
	botUID, err := s.resolveUID(req)
	if err != nil {
		return nil, err
	}
	sess := &Session{
		ID:              randomID(),
		BotUID:          botUID,
		Channel:         req.Channel,
		SrcLang:         req.SrcLang,
		DstLang:         req.DstLang,
		StartedAt:       time.Now().UTC(),
		Status:          "running",
		agoraAppID:      req.AgoraAppID,
		openAIKey:       req.OpenAIKey,
		speakerUID:      req.SpeakerUID,
		idleExitSeconds: req.IdleExitSeconds,
	}
	cmd := s.buildCmd(sess, s.botBinary)
	if cmd.Env == nil {
		cmd.Env = os.Environ()
	}
	cmd.Env = append(cmd.Env, "OPENAI_API_KEY="+sess.openAIKey)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start bot: %w", err)
	}
	sess.cmd = cmd
	s.sessions[sess.ID] = sess
	go s.watchProcess(sess.ID, cmd)
	return sess, nil
}

func (s *Store) watchProcess(id string, cmd *exec.Cmd) {
	err := cmd.Wait()
	code := 0
	if err != nil {
		if ex, ok := err.(*exec.ExitError); ok {
			code = ex.ExitCode()
		}
	}
	s.mu.Lock()
	if sess, ok := s.sessions[id]; ok {
		sess.Status = "exited"
		sess.ExitCode = &code
	}
	s.mu.Unlock()
}

func (s *Store) Stop(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok || sess.Status != "running" {
		return ErrNotFound
	}
	return sess.cmd.Process.Signal(syscall.SIGTERM)
}

func (s *Store) Get(id string) (*Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	return sess, ok
}

func (s *Store) List() []*Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Session, 0)
	for _, sess := range s.sessions {
		if sess.Status == "running" {
			out = append(out, sess)
		}
	}
	return out
}

func (s *Store) resolveUID(req createRequest) (int, error) {
	if req.BotUID != nil {
		for _, sess := range s.sessions {
			if sess.Status == "running" && sess.Channel == req.Channel && sess.BotUID == *req.BotUID {
				return 0, ErrDuplicate
			}
		}
		return *req.BotUID, nil
	}
	used := make(map[int]bool)
	for _, sess := range s.sessions {
		if sess.Status == "running" {
			used[sess.BotUID] = true
		}
	}
	for uid := 2000; uid <= 2999; uid++ {
		if !used[uid] {
			return uid, nil
		}
	}
	return 0, ErrUIDPoolExhausted
}
```

Also remove the placeholder `var _ = syscall.SIGTERM` and `var _ = os.Environ` lines that were added in Task 1.

- [ ] **Step 4: Run store tests — expect pass**

```bash
go test ./cmd/server/ -run TestStore -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/server/store.go cmd/server/main_test.go
git commit -m "feat: implement session store with subprocess lifecycle management"
```

---

### Task 3: /health + auth middleware integration tests

**Files:**
- Modify: `cmd/server/main_test.go` (add TestHealth, TestAuthRequired)

- [ ] **Step 1: Add integration tests for health + auth**

Append to `cmd/server/main_test.go` (replace `var _ = fmt.Sprintf` and `var _ = time.Second` placeholders):

```go
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
```

- [ ] **Step 2: Run tests**

```bash
go test ./cmd/server/ -run "TestHealth|TestAuthRequired" -v
```

Expected: both PASS (handleHealth and authMiddleware already implemented in main.go).

- [ ] **Step 3: Commit**

```bash
git add cmd/server/main_test.go
git commit -m "test: add health + auth integration tests"
```

---

### Task 4: POST /sessions handler

**Files:**
- Modify: `cmd/server/handlers.go` (replace handleCreateSession stub)
- Modify: `cmd/server/main_test.go` (add POST /sessions tests)

- [ ] **Step 1: Add POST /sessions integration tests**

Append to `cmd/server/main_test.go`:

```go
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
```

- [ ] **Step 2: Run tests — expect failures**

```bash
go test ./cmd/server/ -run TestCreateSession -v
```

Expected: FAIL — stub returns 501.

- [ ] **Step 3: Replace handleCreateSession stub in handlers.go**

```go
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
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(sess)
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
```

- [ ] **Step 4: Run tests — expect pass**

```bash
go test ./cmd/server/ -run TestCreateSession -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/server/handlers.go cmd/server/main_test.go
git commit -m "feat: implement POST /sessions handler"
```

---

### Task 5: GET /sessions + GET /sessions/:id handlers

**Files:**
- Modify: `cmd/server/handlers.go` (replace list + get stubs)
- Modify: `cmd/server/main_test.go` (add GET tests)

- [ ] **Step 1: Add GET tests**

Append to `cmd/server/main_test.go`:

```go
func TestListSessions(t *testing.T) {
	srv, _ := newTestServer(t)
	do(t, srv, "POST", "/sessions", validBody())
	resp := do(t, srv, "GET", "/sessions", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var sessions []*Session
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
```

- [ ] **Step 2: Run — expect failures**

```bash
go test ./cmd/server/ -run "TestListSessions|TestGetSession" -v
```

Expected: FAIL — stubs return 501.

- [ ] **Step 3: Replace list + get stubs in handlers.go**

Replace `handleListSessions` and `handleGetSession` (leave others unchanged):

```go
func handleListSessions(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessions := store.List()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sessions)
	}
}

func handleGetSession(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		sess, ok := store.Get(id)
		if !ok {
			writeError(w, http.StatusNotFound, "session not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sess)
	}
}
```

- [ ] **Step 4: Run — expect pass**

```bash
go test ./cmd/server/ -run "TestListSessions|TestGetSession" -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/server/handlers.go cmd/server/main_test.go
git commit -m "feat: implement GET /sessions and GET /sessions/:id handlers"
```

---

### Task 6: DELETE /sessions/:id handler

**Files:**
- Modify: `cmd/server/handlers.go` (replace delete stub)
- Modify: `cmd/server/main_test.go` (add DELETE tests)

- [ ] **Step 1: Add DELETE tests**

Append to `cmd/server/main_test.go`:

```go
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
		return exec.Command("/bin/true")
	}
	req := createRequest{AgoraAppID: "a", OpenAIKey: "k", Channel: "ch", SrcLang: "en", DstLang: "es", IdleExitSeconds: 300}
	sess, err := store.Create(req)
	if err != nil {
		t.Fatal(err)
	}
	// wait for it to exit
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		got, _ := store.Get(sess.ID)
		if got != nil && got.Status == "exited" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	// Stop on exited session should return ErrNotFound
	if err := store.Stop(sess.ID); err != ErrNotFound {
		t.Fatalf("want ErrNotFound on exited session, got %v", err)
	}
}
```

- [ ] **Step 2: Run — expect failures**

```bash
go test ./cmd/server/ -run TestDeleteSession -v
```

Expected: FAIL — stub returns 501.

- [ ] **Step 3: Replace handleDeleteSession stub in handlers.go**

```go
func handleDeleteSession(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if err := store.Stop(id); err != nil {
			writeError(w, http.StatusNotFound, "session not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
```

- [ ] **Step 4: Run all tests**

```bash
go test ./cmd/server/ -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/server/handlers.go cmd/server/main_test.go
git commit -m "feat: implement DELETE /sessions/:id handler; all server tests passing"
```

---

### Task 7: Dockerfile — add Go build stage + new ENTRYPOINT

**Files:**
- Modify: `Dockerfile`

- [ ] **Step 1: Replace Dockerfile**

```dockerfile
# ── Stage 1: Build Go server ───────────────────────────────────────────────────
FROM golang:1.22 AS go-builder
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY cmd/ ./cmd/
RUN CGO_ENABLED=0 GOOS=linux go build -o /server ./cmd/server/

# ── Stage 2: Build C++ bot + runtime image ────────────────────────────────────
FROM ubuntu:22.04

ARG TARGETARCH
ARG AGORA_SDK_VERSION=4.4.32

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y \
    build-essential \
    cmake \
    pkg-config \
    curl \
    libwebsockets-dev \
    libssl-dev \
    libsamplerate0-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY agora_rtc_sdk /app/agora_rtc_sdk

RUN if [ ! -f /app/agora_rtc_sdk/agora_sdk/libagora_rtc_sdk.so ]; then \
        echo "Downloading Agora SDK ${AGORA_SDK_VERSION} for ${TARGETARCH}..." && \
        curl -fsSL \
            "https://github.com/Bac1314/Agora-server-gateway-openai-translate/releases/download/sdk-${AGORA_SDK_VERSION}/Agora_Native_SDK_${TARGETARCH}.tgz" \
            | tar xz -C /app/ \
            && echo "Agora SDK download complete"; \
    else \
        echo "Agora SDK already present (local-dev path)"; \
    fi

RUN mkdir -p /app/agora_rtc_sdk/example/third-party/json_parser/include && \
    curl -fsSL \
    "https://github.com/nlohmann/json/releases/download/v3.11.3/json.hpp" \
    -o /app/agora_rtc_sdk/example/third-party/json_parser/include/json.hpp

WORKDIR /app/agora_rtc_sdk/example
RUN ./build.sh

# Copy Go server binary from go-builder stage
COPY --from=go-builder /server /app/server

ENV LD_LIBRARY_PATH=/app/agora_rtc_sdk/agora_sdk
ENV BOT_BINARY=/app/agora_rtc_sdk/example/out/translator_bot

EXPOSE 8080
WORKDIR /app
ENTRYPOINT ["/app/server"]
```

> Note: `docker-entrypoint.sh` is no longer used. Leave the file in place (it is harmless) but it is no longer referenced.

- [ ] **Step 2: Verify build locally**

```bash
docker build --platform linux/arm64 -t translator-bot-server .
```

Expected: build succeeds, image has `/app/server` and `/app/agora_rtc_sdk/example/out/translator_bot`.

- [ ] **Step 3: Smoke test — server starts**

```bash
docker run --rm -e API_KEY=test -p 8080:8080 translator-bot-server &
sleep 2
curl -s -H "X-Api-Key: test" http://localhost:8080/health
```

Expected: `{"status":"ok","sessions":0}`

- [ ] **Step 4: Commit**

```bash
git add Dockerfile
git commit -m "feat: add Go build stage to Dockerfile, change ENTRYPOINT to REST server"
```

---

### Task 8: render.yaml + CloudFormation update

**Files:**
- Modify: `render.yaml`
- Modify: `deploy/aws/translator-bot.cfn.yaml`

- [ ] **Step 1: Update render.yaml**

Replace `render.yaml` entirely:

```yaml
services:
  - type: web
    name: agora-translator-bot
    runtime: image
    image:
      url: ghcr.io/bac1314/agora-server-gateway-openai-translate:latest
    envVars:
      # Required — set in Render dashboard after deploy
      - key: API_KEY
        sync: false
      # Optional — defaults shown
      - key: PORT
        value: "8080"
      - key: MAX_SESSIONS
        value: "10"
      - key: BOT_BINARY
        value: /app/agora_rtc_sdk/example/out/translator_bot
```

> `OPENAI_API_KEY` and `AGORA_APP_ID` are now supplied per-session in POST /sessions body — not needed as server-level env vars.

- [ ] **Step 2: Update CloudFormation — add ApiKey parameter + port 8080**

In `deploy/aws/translator-bot.cfn.yaml`:

**Add parameter** (after `BotUid` parameter block):

```yaml
  ApiKey:
    Type: String
    NoEcho: true
    Description: Static API key for X-Api-Key header authentication
```

**Update the ECS task definition** container definition to:
- Remove `AGORA_APP_ID` and `OPENAI_API_KEY` environment entries (now per-session)
- Add `API_KEY` and `MAX_SESSIONS` entries
- Add port mapping for 8080

Find the `ContainerDefinitions` section and update the `Environment` and `PortMappings`:

```yaml
              Environment:
                - Name: API_KEY
                  Value: !Ref ApiKey
                - Name: MAX_SESSIONS
                  Value: "10"
                - Name: PORT
                  Value: "8080"
              PortMappings:
                - ContainerPort: 8080
                  Protocol: tcp
```

**Add security group ingress** for port 8080 in the `BotSecurityGroup` resource:

```yaml
      SecurityGroupIngress:
        - IpProtocol: tcp
          FromPort: 8080
          ToPort: 8080
          CidrIp: 0.0.0.0/0
```

- [ ] **Step 3: Commit**

```bash
git add render.yaml deploy/aws/translator-bot.cfn.yaml
git commit -m "feat: update Render + CFN configs for REST server (port 8080, API_KEY)"
```

---

### Task 9: Update CLAUDE.md + README.md

**Files:**
- Modify: `CLAUDE.md`
- Modify: `README.md`

- [ ] **Step 1: Update CLAUDE.md — replace single-session daemon description with REST control plane**

In `CLAUDE.md`, replace the existing "How to build and run" section and add a new "REST control plane" section after "Project layout":

Under **Project layout**, add `cmd/server/` entry:

```
cmd/
  server/
    main.go         HTTP server entry point, auth middleware, /health
    store.go        Session store (in-memory map), subprocess lifecycle
    handlers.go     HTTP handler functions
    main_test.go    Integration tests (httptest + fake-bot subprocess)
go.mod              Go module (stdlib only, no external deps)
```

Replace the **How to build and run** section's "Run" subsection and "CLI flags" table with:

```markdown
### Run (REST server mode)

```bash
docker build --platform linux/arm64 -t translator-bot-server .
docker run --rm \
  -e API_KEY=my-secret-key \
  -p 8080:8080 \
  translator-bot-server
```

### REST API

All requests require `X-Api-Key: <API_KEY>` header.

| Method | Path | Description |
|---|---|---|
| POST | /sessions | Start a translator bot |
| GET | /sessions | List running sessions |
| GET | /sessions/:id | Get session status + exitCode |
| DELETE | /sessions/:id | Stop bot (SIGTERM) |
| GET | /health | Liveness check |

**POST /sessions body:**
```json
{
  "agoraAppId":      "...",
  "openAiKey":       "sk-...",
  "channel":         "my-channel",
  "srcLang":         "en",
  "dstLang":         "es",
  "speakerUid":      0,
  "botUid":          2002,
  "idleExitSeconds": 300
}
```
`botUid` is optional — omit to auto-assign from pool 2000–2999.

### Server environment variables

| Var | Default | Description |
|---|---|---|
| `API_KEY` | (required) | Static key for `X-Api-Key` auth |
| `PORT` | `8080` | HTTP listen port |
| `MAX_SESSIONS` | `10` | Hard cap on concurrent bot processes (returns 503 when full) |
| `BOT_BINARY` | `/app/agora_rtc_sdk/example/out/translator_bot` | Path to bot binary |
```

- [ ] **Step 2: Update README.md — replace single-session run instructions with REST API quick-start**

Replace the "Environment variables / CLI flags" section and "Local development" section with:

```markdown
## Quick start — REST API

```bash
docker build --platform linux/arm64 -t translator-bot-server .
docker run --rm -e API_KEY=my-secret-key -p 8080:8080 translator-bot-server
```

Start a translation session:

```bash
curl -s -X POST http://localhost:8080/sessions \
  -H "X-Api-Key: my-secret-key" \
  -H "Content-Type: application/json" \
  -d '{
    "agoraAppId":  "<your-agora-app-id>",
    "openAiKey":   "sk-...",
    "channel":     "translate-test",
    "srcLang":     "en",
    "dstLang":     "es"
  }'
```

Returns `{"sessionId":"...","botUid":2000,"channel":"translate-test","status":"running",...}`.

Listeners subscribe to `botUid` (auto-assigned from 2000–2999 if omitted).

Stop a session:

```bash
curl -s -X DELETE http://localhost:8080/sessions/<sessionId> \
  -H "X-Api-Key: my-secret-key"
```

## Server environment variables

| Var | Default | Description |
|---|---|---|
| `API_KEY` | (required) | Static key for `X-Api-Key` auth |
| `PORT` | `8080` | HTTP listen port |
| `MAX_SESSIONS` | `10` | Hard cap; returns 503 when full |
| `BOT_BINARY` | `/app/agora_rtc_sdk/example/out/translator_bot` | Path to bot binary |
```

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md README.md
git commit -m "docs: update CLAUDE.md + README.md for REST control plane"
```

---

## Verification

After all tasks complete, run:

```bash
# All tests pass
go test ./cmd/server/ -v

# Docker image builds
docker build --platform linux/arm64 -t translator-bot-server .

# Server starts and health check works
docker run --rm -e API_KEY=test -p 8080:8080 -d translator-bot-server
curl -s -H "X-Api-Key: test" http://localhost:8080/health
# → {"status":"ok","sessions":0}
```
