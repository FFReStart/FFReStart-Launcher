package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FFReStart/FFReStart-Launcher/internal/launch"
	"github.com/FFReStart/FFReStart-Launcher/internal/patch"
)

type panicTokenStore struct{ calls atomic.Int32 }

func (s *panicTokenStore) LoadRefresh(context.Context) (string, error) {
	s.calls.Add(1)
	panic("offline launch read a token")
}

func TestInstalledGamePlayDoesNotWaitForInstallAction(t *testing.T) {
	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		close(requestStarted)
		<-releaseRequest
	}))
	defer server.Close()

	launcher := launch.NewService("game", launch.FuncStarter(func(context.Context, string, ...string) error { return nil }), nil)
	app := NewApp(launcher, nil)
	app.startup(context.Background())
	app.ConfigureInstaller(&patch.Installer{Root: t.TempDir()}, server.URL, server.Client())
	installDone := make(chan error, 1)
	go func() { installDone <- app.InstallOrUpdate() }()
	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("install request did not start")
	}
	begin := time.Now()
	if err := app.PlayOffline(); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(begin); elapsed > 100*time.Millisecond {
		t.Fatalf("offline play waited %s for install action", elapsed)
	}
	close(releaseRequest)
	<-installDone
}
func (s *panicTokenStore) SaveRefresh(context.Context, string) error {
	s.calls.Add(1)
	panic("offline launch created a token")
}

func TestOfflineAppHasNoTokenPath(t *testing.T) {
	store := &panicTokenStore{}
	launcher := launch.NewService("game", launch.FuncStarter(func(context.Context, string, ...string) error { return nil }), nil)
	app := NewApp(launcher, store)
	app.startup(context.Background())
	if err := app.PlayOffline(); err != nil {
		t.Fatal(err)
	}
	if got := store.calls.Load(); got != 0 {
		t.Fatalf("offline launch touched token storage %d times", got)
	}
}
