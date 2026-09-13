package launch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

type blockingChecker struct{ calls atomic.Int32 }

type fakeProcess struct{ wait func() error }

func (p fakeProcess) Wait() error { return p.wait() }

func survivingProcess() Process {
	return fakeProcess{wait: func() error {
		time.Sleep(time.Second)
		return nil
	}}
}

func (c *blockingChecker) Check(ctx context.Context) error {
	c.calls.Add(1)
	<-ctx.Done()
	return ctx.Err()
}

func TestOfflineLaunchDoesNotWaitForUpdate(t *testing.T) {
	checker := &blockingChecker{}
	started := make(chan struct{}, 1)
	svc := NewService("game", FuncStarter(func(context.Context, string, ...string) (Process, error) {
		started <- struct{}{}
		return survivingProcess(), nil
	}), checker)
	svc.SetLaunchGrace(time.Millisecond)
	svc.SetUpdateTimeout(time.Second)
	begin := time.Now()
	if err := svc.PlayOffline(context.Background()); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(begin); elapsed > 100*time.Millisecond {
		t.Fatalf("launch waited %s for background update", elapsed)
	}
	select {
	case <-started:
	default:
		t.Fatal("game was not started")
	}
}

func TestOfflineLaunchChecksAtMostOnce(t *testing.T) {
	checker := &blockingChecker{}
	svc := NewService("game", FuncStarter(func(context.Context, string, ...string) (Process, error) { return survivingProcess(), nil }), checker)
	svc.SetLaunchGrace(time.Millisecond)
	svc.SetUpdateTimeout(20 * time.Millisecond)
	for range 20 {
		if err := svc.PlayOffline(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(30 * time.Millisecond)
	if got := checker.calls.Load(); got != 1 {
		t.Fatalf("update checks = %d, want 1", got)
	}
}

func TestNetworkUnavailableAllowsTwentyOfTwentyLaunches(t *testing.T) {
	var launches atomic.Int32
	checker := UpdateCheckerFunc(func(context.Context) error { return errors.New("network unavailable") })
	svc := NewService("game", FuncStarter(func(context.Context, string, ...string) (Process, error) {
		launches.Add(1)
		return survivingProcess(), nil
	}), checker)
	svc.SetLaunchGrace(time.Millisecond)
	for range 20 {
		if err := svc.PlayOffline(context.Background()); err != nil {
			t.Fatalf("offline launch failed: %v", err)
		}
	}
	if got := launches.Load(); got != 20 {
		t.Fatalf("successful launches = %d, want 20", got)
	}
}

type UpdateCheckerFunc func(context.Context) error

func (f UpdateCheckerFunc) Check(ctx context.Context) error { return f(ctx) }

type installedGamePath string

func (p installedGamePath) CurrentPath() (string, error) { return string(p), nil }

func TestOfflineLaunchUsesInstalledCurrentVersion(t *testing.T) {
	var startedPath string
	svc := NewService("", FuncStarter(func(_ context.Context, path string, _ ...string) (Process, error) {
		startedPath = path
		return survivingProcess(), nil
	}), nil)
	svc.SetLaunchGrace(time.Millisecond)
	svc.SetInstalledGame(installedGamePath(filepath.Join("game", "versions", "2.0.0")), "bin/FFReStart.exe")
	if err := svc.PlayOffline(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("game", "versions", "2.0.0", "bin", "FFReStart.exe")
	if startedPath != want {
		t.Fatalf("started path = %q, want %q", startedPath, want)
	}
}

func TestDevelopmentPathOverridesInstalledVersion(t *testing.T) {
	svc := NewService(filepath.Join("dev", "game.exe"), FuncStarter(func(context.Context, string, ...string) (Process, error) { return survivingProcess(), nil }), nil)
	svc.SetInstalledGame(installedGamePath(filepath.Join("game", "versions", "2.0.0")), "FFReStart.exe")
	resolved, err := svc.ResolvedGamePath()
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.Abs(filepath.Join("dev", "game.exe"))
	if resolved != want {
		t.Fatalf("resolved path = %q, want %q", resolved, want)
	}
}

func TestInstalledGameDiscoversPreferredExecutable(t *testing.T) {
	root := t.TempDir()
	preferred := filepath.Join(root, "FFReStart-Dev-Build", "FFReStart.exe")
	if runtime.GOOS != "windows" {
		preferred = filepath.Join(root, "FFReStart.x86_64")
	}
	if err := os.MkdirAll(filepath.Dir(preferred), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(preferred, []byte("game"), 0o600); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(preferred, 0o700); err != nil {
			t.Fatal(err)
		}
	} // #nosec G302 -- executable test fixture.
	svc := NewService("", FuncStarter(func(context.Context, string, ...string) (Process, error) { return survivingProcess(), nil }), nil)
	svc.SetInstalledGame(installedGamePath(root), "missing-game.exe")
	resolved, err := svc.ResolvedGamePath()
	if err != nil {
		t.Fatal(err)
	}
	if resolved != preferred {
		t.Fatalf("got %q, want %q", resolved, preferred)
	}
}

func TestLaunchSucceedsAfterGracePeriod(t *testing.T) {
	t.Parallel()
	svc := NewService("game", FuncStarter(func(context.Context, string, ...string) (Process, error) {
		return survivingProcess(), nil
	}), nil)
	svc.SetLaunchGrace(10 * time.Millisecond)
	if err := svc.PlayOffline(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultLaunchGraceIsBounded(t *testing.T) {
	t.Parallel()
	svc := NewService("game", FuncStarter(func(context.Context, string, ...string) (Process, error) {
		return survivingProcess(), nil
	}), nil)
	if svc.launchGrace != 5*time.Second {
		t.Fatalf("launch grace = %s, want 5s", svc.launchGrace)
	}
}

func TestLaunchReportsEarlyExit(t *testing.T) {
	t.Parallel()
	for name, waitError := range map[string]error{"clean": nil, "failure": errors.New("exit status 1")} {
		waitError := waitError
		t.Run(name, func(t *testing.T) {
			svc := NewService("game", FuncStarter(func(context.Context, string, ...string) (Process, error) {
				return fakeProcess{wait: func() error { return waitError }}, nil
			}), nil)
			svc.SetLaunchGrace(time.Second)
			if err := svc.PlayOffline(context.Background()); !errors.Is(err, ErrGameExitedEarly) {
				t.Fatalf("error = %v, want ErrGameExitedEarly", err)
			}
		})
	}
}

func TestLaunchReportsStartFailure(t *testing.T) {
	t.Parallel()
	want := errors.New("start failed")
	svc := NewService("game", FuncStarter(func(context.Context, string, ...string) (Process, error) {
		return nil, want
	}), nil)
	if err := svc.PlayOffline(context.Background()); !errors.Is(err, want) {
		t.Fatalf("error = %v, want wrapped start failure", err)
	}
}
