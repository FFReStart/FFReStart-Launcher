package launch

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrGamePathRequired = errors.New("install the game or configure a development path before playing")
	ErrGameExitedEarly  = errors.New("game closed during launch")
)

type InstalledGame interface {
	CurrentPath() (string, error)
}

type Starter interface {
	Start(context.Context, string, ...string) (Process, error)
}

type StdinStarter interface {
	StartWithStdin(context.Context, string, []byte, ...string) (Process, error)
}

type Process interface {
	Wait() error
}

type FuncStarter func(context.Context, string, ...string) (Process, error)

func (f FuncStarter) Start(ctx context.Context, path string, args ...string) (Process, error) {
	return f(ctx, path, args...)
}

type ExecStarter struct{}

func (ExecStarter) Start(_ context.Context, path string, args ...string) (Process, error) {
	// #nosec G204 -- path is the executable explicitly configured by the local user.
	cmd := exec.Command(path, args...)
	cmd.Dir = filepath.Dir(path)
	configureDetachedProcess(cmd)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (ExecStarter) StartWithStdin(_ context.Context, path string, payload []byte, args ...string) (Process, error) {
	// #nosec G204 -- path is the executable explicitly configured by the local user.
	cmd := exec.Command(path, args...)
	cmd.Dir = filepath.Dir(path)
	configureDetachedStdinProcess(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, err
	}
	if _, err := stdin.Write(payload); err != nil {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, fmt.Errorf("write launch hand-off: %w", err)
	}
	if err := stdin.Close(); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, fmt.Errorf("close launch hand-off: %w", err)
	}
	return cmd, nil
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
	launchGrace   time.Duration
}

func NewService(gamePath string, starter Starter, checker UpdateChecker) *Service {
	gamePath = strings.TrimSpace(gamePath)
	if gamePath != "" {
		if absolute, err := filepath.Abs(gamePath); err == nil {
			gamePath = absolute
		}
	}
	return &Service{gamePath: gamePath, starter: starter, checker: checker, updateTimeout: 1500 * time.Millisecond, launchGrace: 5 * time.Second}
}

func (s *Service) SetUpdateTimeout(timeout time.Duration) { s.updateTimeout = timeout }
func (s *Service) SetLaunchGrace(grace time.Duration)     { s.launchGrace = grace }

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
	expected := filepath.Join(root, executable)
	if info, statErr := os.Stat(expected); statErr == nil && !info.IsDir() {
		return expected, nil
	}
	if discovered := discoverExecutable(root); discovered != "" {
		return discovered, nil
	}
	return expected, nil
}

func discoverExecutable(root string) string {
	type candidate struct {
		path      string
		preferred bool
		depth     int
	}
	var candidates []candidate
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		name := strings.ToLower(entry.Name())
		if strings.Contains(name, "launcher") || strings.Contains(name, "unitycrashhandler") {
			return nil
		}
		if runtime.GOOS == "windows" && filepath.Ext(name) != ".exe" {
			return nil
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return nil
		}
		if runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
			return nil
		}
		relative, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		candidates = append(candidates, candidate{path: path, preferred: strings.Contains(name, "ffrestart"), depth: strings.Count(relative, string(filepath.Separator))})
		return nil
	})
	if len(candidates) == 0 {
		return ""
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].preferred != candidates[j].preferred {
			return candidates[i].preferred
		}
		if candidates[i].depth != candidates[j].depth {
			return candidates[i].depth < candidates[j].depth
		}
		return candidates[i].path < candidates[j].path
	})
	return candidates[0].path
}

// PlayOffline starts the game without credentials, tickets, control APIs, or
// keyrings. The update check is best-effort, bounded, process-wide at-most-once,
// and deliberately not awaited by the launch path.
func (s *Service) PlayOffline(ctx context.Context) error {
	path, err := s.ResolvedGamePath()
	if err != nil {
		return err
	}
	process, err := s.starter.Start(ctx, path, "--offline")
	if err != nil {
		return fmt.Errorf("start game: %w", err)
	}
	if process == nil {
		return errors.New("start game: no process handle")
	}
	if err := s.supervise(process); err != nil {
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

// PlayMultiplayer writes exactly one length-delimited LaunchHandoff to stdin.
// The ticket never appears in argv, the environment, logs, or JavaScript.
func (s *Service) PlayMultiplayer(ctx context.Context, ticket []byte, bootstrap LaunchBootstrap) error {
	path, err := s.ResolvedGamePath()
	if err != nil {
		return err
	}
	payload, err := marshalDelimitedHandoff(ticket, bootstrap)
	if err != nil {
		return err
	}
	defer clear(payload)
	starter, ok := s.starter.(StdinStarter)
	if !ok {
		return errors.New("multiplayer stdin hand-off is unavailable")
	}
	process, err := starter.StartWithStdin(ctx, path, payload, "--auth-token-stdin")
	if err != nil {
		return fmt.Errorf("start multiplayer game: %w", err)
	}
	if process == nil {
		return errors.New("start multiplayer game: no process handle")
	}
	return s.supervise(process)
}

func (s *Service) supervise(process Process) error {
	waited := make(chan error, 1)
	go func() { waited <- process.Wait() }()
	timer := time.NewTimer(s.launchGrace)
	defer timer.Stop()
	select {
	case waitErr := <-waited:
		if waitErr != nil {
			return fmt.Errorf("%w: %v", ErrGameExitedEarly, waitErr)
		}
		return ErrGameExitedEarly
	case <-timer.C:
		return nil
	}
}
