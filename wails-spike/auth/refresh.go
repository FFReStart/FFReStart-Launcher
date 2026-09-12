package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func Refresh(ctx context.Context, tokenURL, clientID string, store RefreshStore) (Tokens, error) {
	refresh, err := store.Load()
	if err != nil {
		return Tokens{}, err
	}
	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}, "client_id": {clientID}}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Tokens{}, err
	}
	defer resp.Body.Close()
	var body struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		IDToken      string `json:"id_token"`
		Error        string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Tokens{}, err
	}
	if body.Error == "invalid_grant" {
		_ = store.Clear()
		return Tokens{}, ErrReauthenticationRequired
	}
	if resp.StatusCode != http.StatusOK {
		return Tokens{}, fmt.Errorf("refresh failed: %s", body.Error)
	}
	if body.RefreshToken != "" {
		_ = store.Save(body.RefreshToken)
	}
	return Tokens{body.AccessToken, body.RefreshToken, body.IDToken}, nil
}
