package multiplayer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var ErrUnauthorized = errors.New("multiplayer sign-in expired")

type Config struct {
	BaseURL      string
	BootstrapURL string
	Client       *http.Client
}

type Client struct {
	baseURL      string
	bootstrapURL string
	http         *http.Client
}

type Account struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Profile *struct {
		Handle string `json:"handle"`
	} `json:"profile"`
}

func (a Account) Handle() string {
	if a.Profile == nil || strings.TrimSpace(a.Profile.Handle) == "" {
		return "PLAYER"
	}
	return a.Profile.Handle
}

type Realm struct {
	ID            string `json:"realmId"`
	Name          string `json:"name"`
	Status        string `json:"status"`
	AdmissionOpen bool   `json:"admissionOpen"`
}

type Bootstrap struct {
	RealmID        string `json:"realmId"`
	WorldEndpoint  string `json:"worldEndpoint"`
	GNSCAKeyID     string `json:"gnsCaKeyId"`
	ProtocolMin    uint32 `json:"protocolMin"`
	ProtocolMax    uint32 `json:"protocolMax"`
	ContentVersion string `json:"contentVersion"`
}

type Ticket struct {
	Value     string    `json:"ticket"`
	ExpiresAt time.Time `json:"expiresAt"`
	Bootstrap Bootstrap `json:"bootstrap"`
}

type ProblemError struct {
	Status int
	Code   string
	Detail string
}

func (e *ProblemError) Error() string {
	switch e.Code {
	case "client_not_admitted", "protocol_not_admitted":
		return "The selected server does not accept this game version. Update the game and try again."
	case "launch_not_admitted":
		return "Multiplayer admission was denied. The account may be sanctioned or the realm may be closed."
	case "rate_limit_exceeded":
		return "The server received too many launch requests. Wait briefly and try again."
	}
	if strings.TrimSpace(e.Detail) != "" {
		return e.Detail
	}
	return fmt.Sprintf("multiplayer server returned HTTP %d", e.Status)
}

func New(config Config) (*Client, error) {
	parsed, err := url.Parse(config.BaseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, errors.New("control API URL is invalid")
	}
	client := config.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	bootstrapURL := strings.TrimSpace(config.BootstrapURL)
	if bootstrapURL == "" {
		bootstrapURL = strings.TrimRight(config.BaseURL, "/") + "/v1/bootstrap"
	}
	parsedBootstrap, err := url.Parse(bootstrapURL)
	if err != nil || parsedBootstrap.Scheme == "" || parsedBootstrap.Host == "" {
		return nil, errors.New("bootstrap URL is invalid")
	}
	return &Client{baseURL: strings.TrimRight(config.BaseURL, "/"), bootstrapURL: bootstrapURL, http: client}, nil
}

func (c *Client) Me(ctx context.Context, accessToken string) (Account, error) {
	var account Account
	if err := c.request(ctx, http.MethodGet, "/v1/me", accessToken, nil, &account); err != nil {
		return Account{}, err
	}
	if account.ID == "" || account.Status != "active" {
		return Account{}, errors.New("the multiplayer account is not active")
	}
	return account, nil
}

func (c *Client) Realms(ctx context.Context) ([]Realm, error) {
	var bootstrap struct {
		Realms []Realm `json:"realms"`
	}
	if err := c.requestURL(ctx, http.MethodGet, c.bootstrapURL, "", nil, &bootstrap); err != nil {
		return nil, err
	}
	for _, realm := range bootstrap.Realms {
		if realm.ID == "" || realm.Name == "" {
			return nil, errors.New("the server returned an invalid realm list")
		}
	}
	return bootstrap.Realms, nil
}

func (c *Client) LaunchTicket(ctx context.Context, accessToken, realm string, protocolVersion uint32, buildHash string) (Ticket, error) {
	payload := struct {
		Realm           string `json:"realm"`
		ProtocolVersion uint32 `json:"protocolVersion"`
		BuildHash       string `json:"buildHash"`
	}{realm, protocolVersion, buildHash}
	var ticket Ticket
	if err := c.request(ctx, http.MethodPost, "/v1/launch-tickets", accessToken, payload, &ticket); err != nil {
		return Ticket{}, err
	}
	if err := validateTicket(ticket, realm, protocolVersion); err != nil {
		return Ticket{}, err
	}
	return ticket, nil
}

func validateTicket(ticket Ticket, realm string, protocolVersion uint32) error {
	if ticket.Value == "" || len(ticket.Value) > 64<<10 || ticket.ExpiresAt.IsZero() || ticket.ExpiresAt.Before(time.Now().Add(-15*time.Second)) {
		return errors.New("the server returned an invalid launch ticket")
	}
	bootstrap := ticket.Bootstrap
	if bootstrap.RealmID != realm || bootstrap.WorldEndpoint == "" || bootstrap.GNSCAKeyID == "" || bootstrap.ContentVersion == "" || len(bootstrap.GNSCAKeyID) > 128 || len(bootstrap.ContentVersion) > 256 {
		return errors.New("the server returned an incomplete launch bootstrap")
	}
	host, portText, err := net.SplitHostPort(bootstrap.WorldEndpoint)
	port, portErr := strconv.ParseUint(portText, 10, 16)
	if err != nil || host == "" || portErr != nil || port == 0 {
		return errors.New("the server returned an invalid world endpoint")
	}
	if bootstrap.ProtocolMin == 0 || bootstrap.ProtocolMin > bootstrap.ProtocolMax || protocolVersion < bootstrap.ProtocolMin || protocolVersion > bootstrap.ProtocolMax {
		return errors.New("the server returned an incompatible protocol range")
	}
	return nil
}

func (c *Client) request(ctx context.Context, method, path, accessToken string, input, output any) error {
	return c.requestURL(ctx, method, c.baseURL+path, accessToken, input, output)
}

func (c *Client) requestURL(ctx context.Context, method, endpoint, accessToken string, input, output any) error {
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json, application/problem+json")
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if accessToken != "" {
		request.Header.Set("Authorization", "Bearer "+accessToken)
	}
	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("multiplayer network error: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	data, readErr := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if readErr != nil {
		return errors.New("read multiplayer server response")
	}
	if len(data) > 1<<20 {
		return errors.New("multiplayer server response is too large")
	}
	if response.StatusCode == http.StatusUnauthorized {
		return ErrUnauthorized
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var problem struct {
			Code   string `json:"code"`
			Detail string `json:"detail"`
		}
		_ = json.Unmarshal(data, &problem)
		if accessToken != "" {
			problem.Detail = strings.ReplaceAll(problem.Detail, accessToken, "[redacted]")
		}
		return &ProblemError{Status: response.StatusCode, Code: problem.Code, Detail: problem.Detail}
	}
	if output == nil {
		return nil
	}
	if err := json.Unmarshal(data, output); err != nil {
		return errors.New("the multiplayer server returned malformed JSON")
	}
	return nil
}
