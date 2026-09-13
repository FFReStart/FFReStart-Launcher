package multiplayer

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAccountRealmsAndTicketContract(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/bootstrap" && request.Header.Get("Authorization") != "Bearer access-secret" {
			t.Error("authenticated request omitted bearer token")
		}
		switch request.URL.Path {
		case "/v1/me":
			_ = json.NewEncoder(writer).Encode(map[string]any{"id": "account-id", "status": "active", "profile": map[string]string{"handle": "FusionPilot"}})
		case "/v1/bootstrap":
			_ = json.NewEncoder(writer).Encode(map[string]any{"realms": []map[string]any{{"realmId": "local", "name": "Local", "status": "online", "admissionOpen": true, "worldEndpoint": "127.0.0.1:27020", "gnsCaKeyId": "dev-ca", "protocolMin": 1, "protocolMax": 2, "contentVersion": "dev-content", "updateChannel": "development"}}})
		case "/v1/launch-tickets":
			var body map[string]any
			_ = json.NewDecoder(request.Body).Decode(&body)
			if body["realm"] != "local" || body["protocolVersion"] != float64(1) || body["buildHash"] != strings.Repeat("a", 64) {
				t.Errorf("unexpected ticket request: %#v", body)
			}
			writer.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"ticket": "ticket-secret", "expiresAt": time.Now().Add(time.Minute).UTC(),
				"bootstrap": map[string]any{"realmId": "local", "worldEndpoint": "127.0.0.1:27020", "gnsCaKeyId": "dev-ca", "protocolMin": 1, "protocolMax": 2, "contentVersion": "dev-content"},
			})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL, Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	account, err := client.Me(context.Background(), "access-secret")
	if err != nil || account.Handle() != "FusionPilot" {
		t.Fatalf("account = %+v, error = %v", account, err)
	}
	if realms, err := client.Realms(context.Background()); err != nil || len(realms) != 1 || realms[0].ID != "local" {
		t.Fatalf("realms = %+v, error = %v", realms, err)
	}
	ticket, err := client.LaunchTicket(context.Background(), "access-secret", "local", 1, strings.Repeat("a", 64))
	if err != nil || ticket.Value != "ticket-secret" || ticket.Bootstrap.GNSCAKeyID != "dev-ca" {
		t.Fatalf("ticket = %+v, error = %v", ticket, err)
	}
}

func TestProblemDetailRedactsAccessToken(t *testing.T) {
	t.Parallel()
	const token = "access-secret-that-must-not-cross-js"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(writer).Encode(map[string]string{"code": "custom_denial", "detail": "denied " + token})
	}))
	defer server.Close()
	client, _ := New(Config{BaseURL: server.URL, Client: server.Client()})
	_, err := client.Me(context.Background(), token)
	if err == nil || strings.Contains(err.Error(), token) || !strings.Contains(err.Error(), "[redacted]") {
		t.Fatalf("unsafe error: %v", err)
	}
}

func TestAdmissionProblemsHavePlayerFacingMessages(t *testing.T) {
	t.Parallel()
	for code, phrase := range map[string]string{
		"launch_not_admitted":   "sanctioned or the realm may be closed",
		"client_not_admitted":   "game version",
		"protocol_not_admitted": "game version",
		"rate_limit_exceeded":   "too many launch requests",
	} {
		message := (&ProblemError{Status: http.StatusForbidden, Code: code}).Error()
		if !strings.Contains(message, phrase) {
			t.Fatalf("code %s produced %q", code, message)
		}
	}
}

func TestTicketRejectsIncompleteOrIncompatibleBootstrap(t *testing.T) {
	t.Parallel()
	valid := Ticket{Value: "ticket", ExpiresAt: time.Now().Add(time.Minute), Bootstrap: Bootstrap{RealmID: "local", WorldEndpoint: "127.0.0.1:27020", GNSCAKeyID: "dev-ca", ProtocolMin: 1, ProtocolMax: 2, ContentVersion: "content"}}
	for name, mutate := range map[string]func(*Ticket){
		"missing endpoint": func(ticket *Ticket) { ticket.Bootstrap.WorldEndpoint = "" },
		"wrong realm":      func(ticket *Ticket) { ticket.Bootstrap.RealmID = "other" },
		"bad protocol":     func(ticket *Ticket) { ticket.Bootstrap.ProtocolMin = 2 },
		"missing CA":       func(ticket *Ticket) { ticket.Bootstrap.GNSCAKeyID = "" },
	} {
		t.Run(name, func(t *testing.T) {
			ticket := valid
			mutate(&ticket)
			if err := validateTicket(ticket, "local", 1); err == nil {
				t.Fatal("accepted invalid bootstrap")
			}
		})
	}
}

func TestProblemErrorsExplainAdmissionFailures(t *testing.T) {
	t.Parallel()
	for code, phrase := range map[string]string{"client_not_admitted": "game version", "launch_not_admitted": "sanctioned", "rate_limit_exceeded": "too many"} {
		message := (&ProblemError{Status: 403, Code: code}).Error()
		if !strings.Contains(strings.ToLower(message), strings.ToLower(phrase)) {
			t.Fatalf("%s did not explain %s: %q", code, phrase, message)
		}
	}
}

func TestUnauthorizedIsRefreshableWithoutLeakingToken(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusUnauthorized)
		_, _ = writer.Write([]byte(`{"detail":"access-secret"}`))
	}))
	defer server.Close()
	client, _ := New(Config{BaseURL: server.URL, Client: server.Client()})
	_, err := client.Me(context.Background(), "access-secret")
	if !errors.Is(err, ErrUnauthorized) || strings.Contains(err.Error(), "access-secret") {
		t.Fatalf("unsafe unauthorized error: %v", err)
	}
}
