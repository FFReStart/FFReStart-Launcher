package main

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/FFReStart/FFReStart-Launcher/internal/launch"
)

type panicTokenStore struct{ calls atomic.Int32 }

func (s *panicTokenStore) LoadRefresh(context.Context) (string, error) {
	s.calls.Add(1)
	panic("offline launch read a token")
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
