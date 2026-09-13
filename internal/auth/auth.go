package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type Tokens struct{ AccessToken, RefreshToken, IDToken string }
type BrowserConfig struct {
	Issuer, ClientID, AuthURL, TokenURL string
	Scopes                              []string
	Client                              *http.Client
}
type DeviceConfig struct {
	ClientID, DeviceURL, TokenURL string
	Scopes                        []string
	Client                        *http.Client
	PollInterval                  time.Duration
}
type DevicePrompt struct{ VerificationURI, VerificationURIComplete, UserCode string }

func randomURLSafe(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

// BrowserLogin uses an external browser, loopback redirect, S256 PKCE, state and nonce.
func BrowserLogin(ctx context.Context, config BrowserConfig, open func(string) error) (Tokens, error) {
	if config.Client != nil {
		ctx = context.WithValue(ctx, oauth2.HTTPClient, config.Client)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return Tokens{}, err
	}
	defer func() { _ = listener.Close() }()
	state, err := randomURLSafe(32)
	if err != nil {
		return Tokens{}, err
	}
	nonce, err := randomURLSafe(32)
	if err != nil {
		return Tokens{}, err
	}
	verifier := oauth2.GenerateVerifier()
	redirect := "http://" + listener.Addr().String() + "/callback"
	scopes := config.Scopes
	if len(scopes) == 0 {
		scopes = []string{oidc.ScopeOpenID, "offline_access"}
	}
	oauth := oauth2.Config{ClientID: config.ClientID, RedirectURL: redirect, Endpoint: oauth2.Endpoint{AuthURL: config.AuthURL, TokenURL: config.TokenURL}, Scopes: scopes}
	type callback struct{ code, state string }
	result := make(chan callback, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(writer http.ResponseWriter, request *http.Request) {
		query := request.URL.Query()
		select {
		case result <- callback{query.Get("code"), query.Get("state")}:
		default:
		}
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = writer.Write([]byte("<!doctype html><title>FFReStart sign-in</title><p>Sign-in complete. You may close this window.</p>"))
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	defer func() { _ = server.Shutdown(context.Background()) }()
	go func() { _ = server.Serve(listener) }()
	if err := open(oauth.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier), oauth2.SetAuthURLParam("nonce", nonce))); err != nil {
		return Tokens{}, err
	}
	var received callback
	select {
	case received = <-result:
	case <-ctx.Done():
		return Tokens{}, ctx.Err()
	}
	if received.state != state || received.code == "" {
		return Tokens{}, errors.New("OIDC callback state mismatch or missing code")
	}
	token, err := oauth.Exchange(ctx, received.code, oauth2.VerifierOption(verifier))
	if err != nil {
		return Tokens{}, errors.New("authorization code exchange failed")
	}
	rawID, ok := token.Extra("id_token").(string)
	if !ok || rawID == "" {
		return Tokens{}, errors.New("token response omitted id_token")
	}
	provider, err := oidc.NewProvider(ctx, config.Issuer)
	if err != nil {
		return Tokens{}, fmt.Errorf("OIDC discovery: %w", err)
	}
	idToken, err := provider.Verifier(&oidc.Config{ClientID: config.ClientID}).Verify(ctx, rawID)
	if err != nil {
		return Tokens{}, fmt.Errorf("verify id_token: %w", err)
	}
	var claims struct {
		Nonce string `json:"nonce"`
	}
	if idToken.Claims(&claims) != nil || claims.Nonce != nonce {
		return Tokens{}, errors.New("id_token nonce mismatch")
	}
	return Tokens{token.AccessToken, token.RefreshToken, rawID}, nil
}

func DeviceLogin(ctx context.Context, config DeviceConfig, show func(DevicePrompt) error) (Tokens, error) {
	scopes := config.Scopes
	if len(scopes) == 0 {
		scopes = []string{"openid", "offline_access"}
	}
	client := config.Client
	if client == nil {
		client = http.DefaultClient
	}
	form := url.Values{"client_id": {config.ClientID}, "scope": {strings.Join(scopes, " ")}}
	request, _ := http.NewRequestWithContext(ctx, http.MethodPost, config.DeviceURL, strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := client.Do(request)
	if err != nil {
		return Tokens{}, err
	}
	defer func() { _ = response.Body.Close() }()
	var grant struct {
		DeviceCode              string `json:"device_code"`
		UserCode                string `json:"user_code"`
		VerificationURI         string `json:"verification_uri"`
		VerificationURIComplete string `json:"verification_uri_complete"`
		Interval                int    `json:"interval"`
	}
	if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&grant) != nil || grant.DeviceCode == "" {
		return Tokens{}, errors.New("invalid device authorization response")
	}
	if err := show(DevicePrompt{grant.VerificationURI, grant.VerificationURIComplete, grant.UserCode}); err != nil {
		return Tokens{}, err
	}
	interval := config.PollInterval
	if interval <= 0 {
		interval = time.Duration(grant.Interval) * time.Second
	}
	if interval <= 0 {
		interval = time.Second
	}
	for {
		select {
		case <-ctx.Done():
			return Tokens{}, ctx.Err()
		case <-time.After(interval):
		}
		form = url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:device_code"}, "device_code": {grant.DeviceCode}, "client_id": {config.ClientID}}
		request, _ = http.NewRequestWithContext(ctx, http.MethodPost, config.TokenURL, strings.NewReader(form.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		response, err = client.Do(request)
		if err != nil {
			return Tokens{}, err
		}
		var body struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			IDToken      string `json:"id_token"`
			Error        string `json:"error"`
		}
		err = json.NewDecoder(response.Body).Decode(&body)
		_ = response.Body.Close()
		if err != nil {
			return Tokens{}, err
		}
		if response.StatusCode == http.StatusOK && body.AccessToken != "" {
			return Tokens{body.AccessToken, body.RefreshToken, body.IDToken}, nil
		}
		switch body.Error {
		case "authorization_pending":
			continue
		case "slow_down":
			interval += 5 * time.Second
		default:
			return Tokens{}, fmt.Errorf("device token: %s", body.Error)
		}
	}
}

func PKCEChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
