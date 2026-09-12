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

var ErrReauthenticationRequired = errors.New("fresh sign-in required")

type Tokens struct {
	AccessToken  string
	RefreshToken string
	IDToken      string
}

type BrowserConfig struct {
	Issuer, ClientID, AuthURL, TokenURL string
}

func randomURLSafe(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// BrowserLogin uses the system-browser authorization-code flow. The caller owns
// opening authURL; Wails keeps every token in this Go backend.
func BrowserLogin(ctx context.Context, c BrowserConfig, open func(authURL string) error) (Tokens, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return Tokens{}, err
	}
	defer listener.Close()

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
	oc := oauth2.Config{ClientID: c.ClientID, RedirectURL: redirect, Endpoint: oauth2.Endpoint{AuthURL: c.AuthURL, TokenURL: c.TokenURL}, Scopes: []string{oidc.ScopeOpenID, "offline_access"}}

	type response struct {
		code, state string
		err         error
	}
	result := make(chan response, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		result <- response{code: q.Get("code"), state: q.Get("state")}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("Sign-in complete. You may close this window."))
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	defer server.Shutdown(context.Background())
	go func() { _ = server.Serve(listener) }()

	authURL := oc.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier), oauth2.SetAuthURLParam("nonce", nonce))
	if err := open(authURL); err != nil {
		return Tokens{}, err
	}

	var callback response
	select {
	case callback = <-result:
	case <-ctx.Done():
		return Tokens{}, ctx.Err()
	}
	if callback.state != state || callback.code == "" {
		return Tokens{}, errors.New("OIDC callback state mismatch or missing code")
	}
	token, err := oc.Exchange(ctx, callback.code, oauth2.VerifierOption(verifier))
	if err != nil {
		return Tokens{}, fmt.Errorf("exchange authorization code: %w", err)
	}
	rawID, ok := token.Extra("id_token").(string)
	if !ok || rawID == "" {
		return Tokens{}, errors.New("token response omitted id_token")
	}
	provider, err := oidc.NewProvider(ctx, c.Issuer)
	if err != nil {
		return Tokens{}, fmt.Errorf("OIDC discovery: %w", err)
	}
	idToken, err := provider.Verifier(&oidc.Config{ClientID: c.ClientID}).Verify(ctx, rawID)
	if err != nil {
		return Tokens{}, fmt.Errorf("verify id_token: %w", err)
	}
	var claims struct {
		Nonce string `json:"nonce"`
	}
	if err := idToken.Claims(&claims); err != nil || claims.Nonce != nonce {
		return Tokens{}, errors.New("id_token nonce mismatch")
	}
	return Tokens{AccessToken: token.AccessToken, RefreshToken: token.RefreshToken, IDToken: rawID}, nil
}

type DeviceConfig struct {
	ClientID, DeviceURL, TokenURL string
	PollInterval                  time.Duration
}

type DevicePrompt struct {
	VerificationURI, VerificationURIComplete, UserCode string
}

func DeviceLogin(ctx context.Context, c DeviceConfig, show func(DevicePrompt) error) (Tokens, error) {
	form := url.Values{"client_id": {c.ClientID}, "scope": {"openid offline_access"}}
	resp, err := http.PostForm(c.DeviceURL, form)
	if err != nil {
		return Tokens{}, err
	}
	defer resp.Body.Close()
	var grant struct {
		DeviceCode              string `json:"device_code"`
		UserCode                string `json:"user_code"`
		VerificationURI         string `json:"verification_uri"`
		VerificationURIComplete string `json:"verification_uri_complete"`
		Interval                int    `json:"interval"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&grant) != nil || grant.DeviceCode == "" {
		return Tokens{}, errors.New("invalid device authorization response")
	}
	if err := show(DevicePrompt{grant.VerificationURI, grant.VerificationURIComplete, grant.UserCode}); err != nil {
		return Tokens{}, err
	}
	interval := c.PollInterval
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
		form = url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:device_code"}, "device_code": {grant.DeviceCode}, "client_id": {c.ClientID}}
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.TokenURL, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return Tokens{}, err
		}
		var body struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			IDToken      string `json:"id_token"`
			Error        string `json:"error"`
		}
		err = json.NewDecoder(res.Body).Decode(&body)
		res.Body.Close()
		if err != nil {
			return Tokens{}, err
		}
		if res.StatusCode == http.StatusOK {
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
