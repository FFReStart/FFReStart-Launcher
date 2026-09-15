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
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"
)

func encodedJSON(value any) string {
	data, _ := json.Marshal(value)
	return base64.RawURLEncoding.EncodeToString(data)
}
func signedIDToken(t *testing.T, key *rsa.PrivateKey, issuer, clientID, nonce string) string {
	t.Helper()
	unsigned := encodedJSON(map[string]any{"alg": "RS256", "kid": "test-key", "typ": "JWT"}) + "." + encodedJSON(map[string]any{"iss": issuer, "sub": "player-1", "aud": clientID, "iat": time.Now().Unix(), "exp": time.Now().Add(time.Minute).Unix(), "nonce": nonce})
	hash := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])
	if err != nil {
		t.Fatal(err)
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func TestBrowserLoginUsesLoopbackS256StateAndNonce(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	const clientID = "ffr-launcher"
	var server *httptest.Server
	var nonce, challenge string
	var loopback, s256 atomic.Bool
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(writer).Encode(map[string]any{"issuer": server.URL, "authorization_endpoint": server.URL + "/authorize", "token_endpoint": server.URL + "/token", "jwks_uri": server.URL + "/jwks"})
		case "/jwks":
			_ = json.NewEncoder(writer).Encode(map[string]any{"keys": []any{map[string]any{"kty": "RSA", "kid": "test-key", "alg": "RS256", "use": "sig", "n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())}}})
		case "/authorize":
			query := request.URL.Query()
			nonce = query.Get("nonce")
			challenge = query.Get("code_challenge")
			s256.Store(query.Get("code_challenge_method") == "S256" && challenge != "")
			redirect, _ := url.Parse(query.Get("redirect_uri"))
			loopback.Store(redirect.Hostname() == "127.0.0.1" && redirect.Port() != "")
			values := redirect.Query()
			values.Set("code", "good-code")
			values.Set("state", query.Get("state"))
			redirect.RawQuery = values.Encode()
			http.Redirect(writer, request, redirect.String(), http.StatusFound) // #nosec G710 -- test follows the generated loopback URL.
		case "/token":
			_ = request.ParseForm()
			if PKCEChallenge(request.Form.Get("code_verifier")) != challenge {
				http.Error(writer, `{"error":"invalid_grant"}`, http.StatusBadRequest)
				return
			}
			writer.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(writer).Encode(map[string]any{"access_token": "access", "refresh_token": "refresh", "token_type": "Bearer", "expires_in": 300, "id_token": signedIDToken(t, key, server.URL, clientID, nonce)})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tokens, err := BrowserLogin(ctx, BrowserConfig{Issuer: server.URL, ClientID: clientID, AuthURL: server.URL + "/authorize", TokenURL: server.URL + "/token", Client: server.Client()}, func(location string) error {
		go func() {
			response, _ := http.Get(location) // #nosec G107 -- test server supplies this local URL.
			if response != nil {
				_ = response.Body.Close()
			}
		}()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if tokens.RefreshToken != "refresh" || !loopback.Load() || !s256.Load() || nonce == "" {
		t.Fatal("browser flow omitted required security evidence")
	}
}

func TestDeviceCodePolling(t *testing.T) {
	var polls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/device":
			_ = json.NewEncoder(writer).Encode(map[string]any{"device_code": "secret", "user_code": "FUSION", "verification_uri": "https://example.test/device", "interval": 1})
		case "/token":
			if polls.Add(1) == 1 {
				writer.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(writer).Encode(map[string]string{"error": "authorization_pending"})
				return
			}
			_ = json.NewEncoder(writer).Encode(map[string]string{"access_token": "access", "refresh_token": "refresh"})
		}
	}))
	defer server.Close()
	var prompt DevicePrompt
	tokens, err := DeviceLogin(context.Background(), DeviceConfig{ClientID: "ffr-launcher", DeviceURL: server.URL + "/device", TokenURL: server.URL + "/token", Client: server.Client(), PollInterval: time.Millisecond}, func(value DevicePrompt) error { prompt = value; return nil })
	if err != nil || prompt.UserCode != "FUSION" || tokens.RefreshToken != "refresh" || polls.Load() != 2 {
		t.Fatalf("unexpected device flow: %#v %#v %v", prompt, tokens, err)
	}
}

type unavailableStore struct{}

func (unavailableStore) Save(string) error     { return errors.New("no keyring") }
func (unavailableStore) Load() (string, error) { return "", errors.New("no keyring") }
func (unavailableStore) Clear() error          { return nil }
func TestMemoryOnlyFallback(t *testing.T) {
	store := &FallbackStore{Primary: unavailableStore{}, Memory: &MemoryStore{}}
	if err := store.Save("memory-only"); err != nil {
		t.Fatal(err)
	}
	if value, _ := store.Load(); value != "memory-only" {
		t.Fatalf("got %q", value)
	}
}

func TestInvalidGrantClearsRefreshToken(t *testing.T) {
	store := &MemoryStore{}
	_ = store.Save("expired-family-token")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = writer.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer server.Close()
	_, err := RefreshWithClient(context.Background(), server.URL, "ffr-launcher", store, server.Client())
	if !errors.Is(err, ErrReauthenticationRequired) {
		t.Fatalf("got %v", err)
	}
	if _, err := store.Load(); err == nil {
		t.Fatal("refresh token was not cleared")
	}
}

func TestRefreshKeepsTokenWhenRotationResponseOmitsSuccessor(t *testing.T) {
	t.Parallel()
	store := &MemoryStore{}
	_ = store.Save("current-refresh")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(writer).Encode(map[string]string{"access_token": "new-access"})
	}))
	defer server.Close()
	tokens, err := RefreshWithClient(context.Background(), server.URL, "launcher", store, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	stored, err := store.Load()
	if err != nil || tokens.RefreshToken != "current-refresh" || stored != "current-refresh" {
		t.Fatalf("refresh token was lost: tokens=%+v stored=%q err=%v", tokens, stored, err)
	}
}
