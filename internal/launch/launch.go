package launch

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var ErrGamePathRequired = errors.New("configure the game executable before playing")

type Starter interface {
	Start(context.Context, string, ...string) error
}

type FuncStarter func(context.Context, string, ...string) error

func (f FuncStarter) Start(ctx context.Context, path string, args ...string) error {
	return f(ctx, path, args...)
}

type ExecStarter struct{}

func (ExecStarter) Start(_ context.Context, path string, args ...string) error {
	// #nosec G204 -- path is the executable explicitly configured by the local user.
	cmd := exec.Command(path, args...)
	return cmd.Start()
}

type UpdateChecker interface {
	Check(context.Context) error
}

type Service struct {
	mu            sync.RWMutex
	gamePath      string
	starter       Starter
	checker       UpdateChecker
	updateOnce    sync.Once
	updateTimeout time.Duration
}

func NewService(gamePath string, starter Starter, checker UpdateChecker) *Service {
	return &Service{gamePath: strings.TrimSpace(gamePath), starter: starter, checker: checker, updateTimeout: 1500 * time.Millisecond}
}

func (s *Service) SetUpdateTimeout(timeout time.Duration) { s.updateTimeout = timeout }

func (s *Service) GamePath() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.gamePath
}

func (s *Service) SetGamePath(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return ErrGamePathRequired
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.gamePath = abs
	s.mu.Unlock()
	return nil
}

// PlayOffline starts the game without credentials, tickets, control APIs, or
// keyrings. The update check is best-effort, bounded, process-wide at-most-once,
// and deliberately not awaited by the launch path.
func (s *Service) PlayOffline(ctx context.Context) error {
	path := s.GamePath()
	if path == "" {
		return ErrGamePathRequired
	}
	if err := s.starter.Start(ctx, path, "--offline"); err != nil {
		return err
	}
	s.updateOnce.Do(func() {
		if s.checker == nil {
			return
		}
		go func() {
			checkCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.updateTimeout)
			defer cancel()
			_ = s.checker.Check(checkCtx)
		}()
	})
	return nil
}
