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

func randomID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return fmt.Sprintf("%x", b)
}
