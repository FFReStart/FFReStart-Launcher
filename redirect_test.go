package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/FFReStart/FFReStart-Launcher/internal/auth"
	"github.com/FFReStart/FFReStart-Launcher/internal/multiplayer"
)

func TestControlAPIRedirectDoesNotForwardCredentials(t *testing.T) {
	t.Parallel()
	for _, status := range []int{http.StatusFound, http.StatusTemporaryRedirect} {
		status := status
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()
			var targetCalls atomic.Int32
			target := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
				targetCalls.Add(1)
				body, _ := io.ReadAll(request.Body)
				t.Errorf("redirect target received authorization %q and body %q", request.Header.Get("Authorization"), body)
			}))
			defer target.Close()
			source := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				http.Redirect(writer, request, target.URL, status)
			}))
			defer source.Close()

			client, err := multiplayer.New(multiplayer.Config{BaseURL: source.URL, Client: newIdentityHTTPClient()})
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.LaunchTicket(context.Background(), "access-secret", "local", 1, strings.Repeat("a", 64))
			if err == nil {
				t.Fatal("control API redirect succeeded")
			}
			if targetCalls.Load() != 0 {
				t.Fatalf("redirect target received %d requests", targetCalls.Load())
			}
		})
	}
}

func TestRefreshRedirectDoesNotForwardToken(t *testing.T) {
	t.Parallel()
	for _, status := range []int{http.StatusFound, http.StatusTemporaryRedirect} {
		status := status
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()
			var targetCalls atomic.Int32
			target := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
				targetCalls.Add(1)
				body, _ := io.ReadAll(request.Body)
				t.Errorf("redirect target received authorization %q and body %q", request.Header.Get("Authorization"), body)
			}))
			defer target.Close()
			source := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				http.Redirect(writer, request, target.URL, status)
			}))
			defer source.Close()

			store := &auth.MemoryStore{}
			if err := store.Save("refresh-secret"); err != nil {
				t.Fatal(err)
			}
			if _, err := auth.RefreshWithClient(context.Background(), source.URL, "launcher", store, newIdentityHTTPClient()); err == nil {
				t.Fatal("refresh redirect succeeded")
			}
			if targetCalls.Load() != 0 {
				t.Fatalf("redirect target received %d requests", targetCalls.Load())
			}
		})
	}
}
