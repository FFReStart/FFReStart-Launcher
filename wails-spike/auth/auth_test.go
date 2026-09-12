package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	keyring "github.com/zalando/go-keyring"
)

func b64JSON(v any) string { b, _ := json.Marshal(v); return base64.RawURLEncoding.EncodeToString(b) }

func signIDToken(t *testing.T, key *rsa.PrivateKey, issuer, clientID, nonce string) string {
	t.Helper()
	header := b64JSON(map[string]any{"alg": "RS256", "kid": "spike-key", "typ": "JWT"})
	claims := b64JSON(map[string]any{"iss": issuer, "sub": "player-1", "aud": clientID, "iat": time.Now().Unix(), "exp": time.Now().Add(time.Minute).Unix(), "nonce": nonce})
	unsigned := header + "." + claims
	h := sha256.Sum256([]byte(unsigned))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, h[:])
	if err != nil {
		t.Fatal(err)
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func TestBrowserLoginUsesLoopbackS256StateAndNonce(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	const clientID = "ffr-launcher"
	var provider *httptest.Server
	var requestedNonce, expectedChallenge string
	var sawLoopback, sawS256 atomic.Bool
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			json.NewEncoder(w).Encode(map[string]any{"issuer": provider.URL, "authorization_endpoint": provider.URL + "/authorize", "token_endpoint": provider.URL + "/token", "jwks_uri": provider.URL + "/jwks"})
		case "/jwks":
			n := base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes())
			e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes())
			json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]any{"kty": "RSA", "kid": "spike-key", "alg": "RS256", "use": "sig", "n": n, "e": e}}})
		case "/authorize":
			q := r.URL.Query()
			requestedNonce = q.Get("nonce")
			expectedChallenge = q.Get("code_challenge")
			sawS256.Store(q.Get("code_challenge_method") == "S256" && expectedChallenge != "")
			redirect, _ := url.Parse(q.Get("redirect_uri"))
			sawLoopback.Store(redirect.Hostname() == "127.0.0.1" && redirect.Port() != "")
			v := redirect.Query()
			v.Set("code", "good-code")
			v.Set("state", q.Get("state"))
			redirect.RawQuery = v.Encode()
			http.Redirect(w, r, redirect.String(), http.StatusFound)
		case "/token":
			_ = r.ParseForm()
			if r.Form.Get("code") != "good-code" || PKCEChallenge(r.Form.Get("code_verifier")) != expectedChallenge {
				http.Error(w, `{"error":"invalid_grant"}`, 400)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"access_token": "access", "refresh_token": "refresh", "token_type": "Bearer", "expires_in": 300, "id_token": signIDToken(t, key, provider.URL, clientID, requestedNonce)})
		default:
			http.NotFound(w, r)
		}
	})
	provider = httptest.NewServer(h)
	defer provider.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tokens, err := BrowserLogin(ctx, BrowserConfig{Issuer: provider.URL, ClientID: clientID, AuthURL: provider.URL + "/authorize", TokenURL: provider.URL + "/token"}, func(u string) error {
		go func() {
			resp, _ := http.Get(u)
			if resp != nil {
				resp.Body.Close()
			}
		}()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if tokens.RefreshToken != "refresh" || !sawLoopback.Load() || !sawS256.Load() || requestedNonce == "" {
		t.Fatalf("flow evidence missing: %+v", tokens)
	}
}

func TestDeviceCodePolling(t *testing.T) {
	var polls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/device":
			json.NewEncoder(w).Encode(map[string]any{"device_code": "device-secret", "user_code": "FUSION", "verification_uri": "https://example.test/device", "verification_uri_complete": "https://example.test/device?user_code=FUSION", "expires_in": 600, "interval": 1})
		case "/token":
			_ = r.ParseForm()
			if r.Form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:device_code" {
				t.Errorf("wrong grant type")
			}
			if polls.Add(1) == 1 {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"access_token": "device-access", "refresh_token": "device-refresh", "token_type": "Bearer"})
		}
	}))
	defer server.Close()
	var prompt DevicePrompt
	tokens, err := DeviceLogin(context.Background(), DeviceConfig{ClientID: "ffr-launcher", DeviceURL: server.URL + "/device", TokenURL: server.URL + "/token", PollInterval: time.Millisecond}, func(p DevicePrompt) error { prompt = p; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if prompt.UserCode != "FUSION" || tokens.RefreshToken != "device-refresh" || polls.Load() != 2 {
		t.Fatalf("unexpected result: %#v %#v", prompt, tokens)
	}
}

func TestInvalidGrantClearsRefreshToken(t *testing.T) {
	store := &MemoryStore{}
	_ = store.Save("stolen-family-token")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer server.Close()
	_, err := Refresh(context.Background(), server.URL, "ffr-launcher", store)
	if !errors.Is(err, ErrReauthenticationRequired) {
		t.Fatalf("got %v", err)
	}
	if _, err := store.Load(); !errors.Is(err, keyring.ErrNotFound) {
		t.Fatal("refresh token was not cleared")
	}
}

type brokenStore struct{}

func (brokenStore) Save(string) error     { return errors.New("no keychain") }
func (brokenStore) Load() (string, error) { return "", errors.New("no keychain") }
func (brokenStore) Clear() error          { return nil }

func TestMemoryOnlyFallback(t *testing.T) {
	f := &FallbackStore{Primary: brokenStore{}, Memory: &MemoryStore{}}
	if err := f.Save("memory-only"); err != nil {
		t.Fatal(err)
	}
	if got, _ := f.Load(); got != "memory-only" {
		t.Fatalf("got %q", got)
	}
}

func TestWindowsCredentialManagerRoundTripAndCleanup(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows Credential Manager proof")
	}
	store := KeyringStore{Service: KeyringService, User: fmt.Sprintf("test-%d", time.Now().UnixNano())}
	defer store.Clear()
	if err := store.Save("temporary-refresh-token"); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load()
	if err != nil || !strings.EqualFold(got, "temporary-refresh-token") {
		t.Fatalf("credential round trip: %q %v", got, err)
	}
	if err := store.Clear(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); !errors.Is(err, keyring.ErrNotFound) {
		t.Fatalf("credential was not deleted: %v", err)
	}
}
