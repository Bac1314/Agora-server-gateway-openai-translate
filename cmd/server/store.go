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
	return nil
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
