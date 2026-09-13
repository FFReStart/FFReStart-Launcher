package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

var ErrReauthenticationRequired = errors.New("fresh sign-in required")

func Refresh(ctx context.Context, tokenURL, clientID string, store RefreshStore) (Tokens, error) {
	refreshToken, err := store.Load()
	if err != nil {
		return Tokens{}, err
	}
	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refreshToken}, "client_id": {clientID}}
	request, _ := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return Tokens{}, err
	}
	defer func() { _ = response.Body.Close() }()
	var body struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		IDToken      string `json:"id_token"`
		Error        string `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		return Tokens{}, err
	}
	if body.Error == "invalid_grant" {
		_ = store.Clear()
		return Tokens{}, ErrReauthenticationRequired
	}
	if response.StatusCode != http.StatusOK {
		return Tokens{}, fmt.Errorf("refresh failed: %s", body.Error)
	}
	if body.RefreshToken != "" {
		_ = store.Save(body.RefreshToken)
	}
	return Tokens{body.AccessToken, body.RefreshToken, body.IDToken}, nil
}
