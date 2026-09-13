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

func (c *blockingChecker) Check(ctx context.Context) error {
	c.calls.Add(1)
	<-ctx.Done()
	return ctx.Err()
}

func TestOfflineLaunchDoesNotWaitForUpdate(t *testing.T) {
	checker := &blockingChecker{}
	started := make(chan struct{}, 1)
	svc := NewService("game", FuncStarter(func(context.Context, string, ...string) error { started <- struct{}{}; return nil }), checker)
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
	svc := NewService("game", FuncStarter(func(context.Context, string, ...string) error { return nil }), checker)
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
	svc := NewService("game", FuncStarter(func(context.Context, string, ...string) error { launches.Add(1); return nil }), checker)
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
	svc := NewService("", FuncStarter(func(_ context.Context, path string, _ ...string) error {
		startedPath = path
		return nil
	}), nil)
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
	svc := NewService(filepath.Join("dev", "game.exe"), FuncStarter(func(context.Context, string, ...string) error { return nil }), nil)
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
	svc := NewService("", FuncStarter(func(context.Context, string, ...string) error { return nil }), nil)
	svc.SetInstalledGame(installedGamePath(root), "missing-game.exe")
	resolved, err := svc.ResolvedGamePath()
	if err != nil {
		t.Fatal(err)
	}
	if resolved != preferred {
		t.Fatalf("got %q, want %q", resolved, preferred)
	}
}
