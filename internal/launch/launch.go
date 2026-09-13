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

var ErrGamePathRequired = errors.New("install the game or configure a development path before playing")

type InstalledGame interface {
	CurrentPath() (string, error)
}

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
	installedGame InstalledGame
	executable    string
	starter       Starter
	checker       UpdateChecker
	updateOnce    sync.Once
	updateTimeout time.Duration
}

func NewService(gamePath string, starter Starter, checker UpdateChecker) *Service {
	gamePath = strings.TrimSpace(gamePath)
	if gamePath != "" {
		if absolute, err := filepath.Abs(gamePath); err == nil {
			gamePath = absolute
		}
	}
	return &Service{gamePath: gamePath, starter: starter, checker: checker, updateTimeout: 1500 * time.Millisecond}
}

func (s *Service) SetUpdateTimeout(timeout time.Duration) { s.updateTimeout = timeout }

// SetInstalledGame makes the patch installer's current immutable version the
// default. A configured game path remains a development override.
func (s *Service) SetInstalledGame(game InstalledGame, executable string) {
	s.mu.Lock()
	s.installedGame = game
	s.executable = filepath.Clean(strings.TrimSpace(executable))
	s.mu.Unlock()
}

func (s *Service) GamePath() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.gamePath
}

func (s *Service) SetGamePath(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		s.mu.Lock()
		s.gamePath = ""
		s.mu.Unlock()
		return nil
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

// ResolvedGamePath returns the development override or the executable inside
// the patch installer's currently selected version directory.
func (s *Service) ResolvedGamePath() (string, error) {
	s.mu.RLock()
	override, installed, executable := s.gamePath, s.installedGame, s.executable
	s.mu.RUnlock()
	if override != "" {
		return override, nil
	}
	if installed == nil || executable == "" || filepath.IsAbs(executable) || executable == "." || executable == ".." || strings.HasPrefix(executable, ".."+string(filepath.Separator)) {
		return "", ErrGamePathRequired
	}
	root, err := installed.CurrentPath()
	if err != nil || root == "" {
		return "", ErrGamePathRequired
	}
	return filepath.Join(root, executable), nil
}

// PlayOffline starts the game without credentials, tickets, control APIs, or
// keyrings. The update check is best-effort, bounded, process-wide at-most-once,
// and deliberately not awaited by the launch path.
func (s *Service) PlayOffline(ctx context.Context) error {
	path, err := s.ResolvedGamePath()
	if err != nil {
		return err
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
