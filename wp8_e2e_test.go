package main

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FFReStart/FFReStart-Launcher/internal/auth"
	"github.com/FFReStart/FFReStart-Launcher/internal/launch"
)

type e2eJSON map[string]any

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *lockedBuffer) Write(value []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(value)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

type e2eDummyStarter struct {
	command *exec.Cmd
	output  lockedBuffer
	argv    []string
	ticket  string
}

func (s *e2eDummyStarter) Start(context.Context, string, ...string) (launch.Process, error) {
	return nil, errors.New("offline start is not used by the WP8 end-to-end test")
}

func (s *e2eDummyStarter) StartWithStdin(ctx context.Context, path string, payload []byte, args ...string) (launch.Process, error) {
	s.argv = append([]string(nil), args...)
	message, err := delimitedMessage(payload)
	if err != nil {
		return nil, err
	}
	s.ticket = string(testProtobufBytesField(message, 1))
	commandArgs := append([]string{"-test.run=^TestWP8DummyGameProcess$", "--"}, args...)
	command := exec.CommandContext(ctx, path, commandArgs...) // #nosec G204 -- current signed test binary and fixed arguments.
	command.Stdin = bytes.NewReader(payload)
	command.Stdout = &s.output
	command.Stderr = &s.output
	if err := command.Start(); err != nil {
		return nil, err
	}
	s.command = command
	return command, nil
}

func TestWP8DockerEndToEnd(t *testing.T) {
	configPath := os.Getenv("FFRESTART_E2E_CONFIG")
	credentialPath := os.Getenv("FFRESTART_E2E_PLAYER")
	adminKeyPath := os.Getenv("FFRESTART_E2E_ADMIN_KEY")
	loginTokenPath := os.Getenv("FFRESTART_E2E_LOGIN_TOKEN")
	internalOrigin := os.Getenv("FFRESTART_E2E_ZITADEL_INTERNAL")
	if configPath == "" || credentialPath == "" || adminKeyPath == "" || loginTokenPath == "" || internalOrigin == "" {
		t.Skip("Docker WP8 environment is not configured")
	}
	configuration, err := loadServerConfigurationWithLookup(configPath, func(host string) ([]net.IP, error) {
		if host == "auth.ffrestart.test" {
			return []net.IP{net.ParseIP("127.0.0.1")}, nil
		}
		return net.LookupIP(host)
	})
	if err != nil {
		t.Fatal(err)
	}
	player := readE2EObject(t, credentialPath)
	issuerURL, err := url.Parse(configuration.Issuer)
	if err != nil {
		t.Fatal(err)
	}
	internalURL, err := url.Parse(internalOrigin)
	if err != nil {
		t.Fatal(err)
	}
	client := mappedIssuerClient(issuerURL.Host, internalURL.Host)
	adminToken := e2eAdminToken(t, client, internalOrigin, configuration.Issuer, adminKeyPath)
	session := e2eRPC(t, client, internalOrigin, issuerURL.Host, "/zitadel.session.v2.SessionService/CreateSession", adminToken, e2eJSON{
		"checks": e2eJSON{
			"user":     e2eJSON{"userId": requiredE2EString(t, player, "userId")},
			"password": e2eJSON{"password": requiredE2EString(t, player, "password")},
		},
		"lifetime": "900s",
	})
	loginTokenBytes, err := os.ReadFile(loginTokenPath) // #nosec G304,G703 -- explicit ignored dev-stack credential path.
	if err != nil {
		t.Fatal(err)
	}
	loginToken := strings.TrimSpace(string(loginTokenBytes))
	clear(loginTokenBytes)

	starter := &e2eDummyStarter{}
	launcher := launch.NewService(os.Args[0], starter, nil)
	launcher.SetLaunchGrace(200 * time.Millisecond)
	settings := defaultSettings(t.TempDir())
	settings.Server = configuration
	app := NewApp(launcher, nil)
	app.ConfigureExperience(&settingsStore{path: filepath.Join(t.TempDir(), "settings.json")}, settings, settings.InstallDirectory, &auth.MemoryStore{})
	app.ConfigureInstaller(nil, "", client)
	app.ConfigureIdentityClient(client)
	app.ConfigureMultiplayerBuild(1, strings.Repeat("0", 64))
	app.startup(context.Background())
	app.openBrowser = func(location string) error {
		response, err := client.Get(location) // #nosec G107 -- URL was constructed by the configured loopback-mapped issuer.
		if err != nil {
			return err
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusFound && response.StatusCode != http.StatusSeeOther {
			return fmt.Errorf("authorization returned HTTP %d", response.StatusCode)
		}
		loginLocation := response.Header.Get("Location")
		authRequest := ""
		if parsed, parseErr := url.Parse(loginLocation); parseErr == nil {
			authRequest = parsed.Query().Get("authRequest")
		}
		if authRequest == "" {
			return errors.New("authorization response omitted authRequest")
		}
		callback := e2eRPC(t, client, internalOrigin, issuerURL.Host, "/zitadel.oidc.v2.OIDCService/CreateCallback", loginToken, e2eJSON{
			"authRequestId": authRequest,
			"session":       e2eJSON{"sessionId": requiredE2EString(t, session, "sessionId"), "sessionToken": requiredE2EString(t, session, "sessionToken")},
		})
		callbackResponse, err := client.Get(requiredE2EString(t, callback, "callbackUrl")) // #nosec G107 -- ZITADEL returns the launcher's bound loopback callback.
		if err == nil {
			_ = callbackResponse.Body.Close()
		}
		return err
	}

	if err := app.SignInBrowser(false); err != nil {
		t.Fatal(err)
	}
	state := app.GetLauncherState()
	if !state.SignedIn || state.AccountID == "" || state.SelectedRealm == "" {
		t.Fatalf("launcher did not establish the real account and realm: %+v", state)
	}
	if err := app.PlayMultiplayer(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if starter.command != nil && starter.command.Process != nil {
			_ = starter.command.Process.Kill()
		}
	}()
	if strings.Join(starter.argv, " ") != "--auth-token-stdin" || strings.Contains(strings.Join(starter.argv, " "), starter.ticket) {
		t.Fatalf("unsafe game argv: %#v", starter.argv)
	}
	deadline := time.Now().Add(2 * time.Second)
	for !strings.Contains(starter.output.String(), "\"realm\"") && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	output := strings.TrimSpace(starter.output.String())
	if !strings.Contains(output, "\"realm\":") || !strings.Contains(output, "\"account\":") || strings.Contains(output, starter.ticket) {
		t.Fatalf("dummy game produced unsafe or incomplete output %q", output)
	}
}

func TestWP8DummyGameProcess(t *testing.T) {
	if !containsArgument(os.Args, "--auth-token-stdin") {
		return
	}
	payload, err := io.ReadAll(io.LimitReader(os.Stdin, 1<<20))
	if err != nil {
		os.Exit(2)
	}
	message, err := delimitedMessage(payload)
	if err != nil {
		os.Exit(3)
	}
	ticket := string(testProtobufBytesField(message, 1))
	bootstrap := testProtobufBytesField(message, 2)
	realm := string(testProtobufBytesField(bootstrap, 1))
	parts := strings.Split(ticket, ".")
	if len(parts) != 3 {
		os.Exit(4)
	}
	claimsBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		os.Exit(5)
	}
	var claims e2eJSON
	if json.Unmarshal(claimsBytes, &claims) != nil {
		os.Exit(6)
	}
	_ = json.NewEncoder(os.Stdout).Encode(e2eJSON{"realm": realm, "account": claims["sub"]})
	clear(payload)
	clear(claimsBytes)
	time.Sleep(2 * time.Second)
	os.Exit(0)
}

func mappedIssuerClient(issuerAddress, internalAddress string) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if strings.EqualFold(address, issuerAddress) {
			address = internalAddress
		}
		return dialer.DialContext(ctx, network, address)
	}
	return &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func e2eAdminToken(t *testing.T, client *http.Client, internalOrigin, issuer, keyPath string) string {
	t.Helper()
	credential := readE2EObject(t, keyPath)
	block, _ := pem.Decode([]byte(requiredE2EString(t, credential, "key")))
	if block == nil {
		t.Fatal("admin key is malformed")
	}
	var key *rsa.PrivateKey
	if parsed, parseErr := x509.ParsePKCS8PrivateKey(block.Bytes); parseErr == nil {
		key, _ = parsed.(*rsa.PrivateKey)
	} else {
		parsedPKCS1, pkcs1Err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if pkcs1Err != nil {
			t.Fatal(pkcs1Err)
		}
		key = parsedPKCS1
	}
	if key == nil {
		t.Fatal("admin key is not RSA")
	}
	now := time.Now().Unix()
	unsigned := encodeE2EJSON(e2eJSON{"alg": "RS256", "kid": requiredE2EString(t, credential, "keyId"), "typ": "JWT"}) + "." + encodeE2EJSON(e2eJSON{"iss": requiredE2EString(t, credential, "userId"), "sub": requiredE2EString(t, credential, "userId"), "aud": issuer, "iat": now, "exp": now + 300})
	hash := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])
	if err != nil {
		t.Fatal(err)
	}
	assertion := unsigned + "." + base64.RawURLEncoding.EncodeToString(signature)
	form := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "scope": {"openid urn:zitadel:iam:org:project:id:zitadel:aud"}, "assertion": {assertion}}
	request, _ := http.NewRequest(http.MethodPost, internalOrigin+"/oauth/v2/token", strings.NewReader(form.Encode())) // #nosec G704 -- explicit local E2E origin.
	request.Host = mustE2EURL(t, issuer).Host
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := client.Do(request) // #nosec G704 -- explicit local E2E origin.
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	var token e2eJSON
	if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&token) != nil {
		t.Fatalf("admin token request returned HTTP %d", response.StatusCode)
	}
	return requiredE2EString(t, token, "access_token")
}

func e2eRPC(t *testing.T, client *http.Client, internalOrigin, host, path, token string, body e2eJSON) e2eJSON {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request, _ := http.NewRequest(http.MethodPost, internalOrigin+path, bytes.NewReader(encoded)) // #nosec G704 -- explicit local E2E origin and fixed RPC path.
	request.Host = host
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Connect-Protocol-Version", "1")
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request) // #nosec G704 -- explicit local E2E origin.
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	var result e2eJSON
	if response.StatusCode < 200 || response.StatusCode >= 300 || json.NewDecoder(response.Body).Decode(&result) != nil {
		t.Fatalf("identity RPC returned HTTP %d", response.StatusCode)
	}
	return result
}

func readE2EObject(t *testing.T, path string) e2eJSON {
	t.Helper()
	data, err := os.ReadFile(path) // #nosec G304,G703 -- explicit ignored dev-stack fixture path.
	if err != nil {
		t.Fatal(err)
	}
	defer clear(data)
	var result e2eJSON
	if json.Unmarshal(data, &result) != nil {
		t.Fatal("development fixture is malformed")
	}
	return result
}

func requiredE2EString(t *testing.T, value e2eJSON, key string) string {
	t.Helper()
	result, ok := value[key].(string)
	if !ok || result == "" {
		t.Fatalf("development fixture omitted %s", key)
	}
	return result
}

func encodeE2EJSON(value e2eJSON) string {
	data, _ := json.Marshal(value)
	return base64.RawURLEncoding.EncodeToString(data)
}

func mustE2EURL(t *testing.T, value string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func containsArgument(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func delimitedMessage(payload []byte) ([]byte, error) {
	length, read := binary.Uvarint(payload)
	if read <= 0 || length > uint64(len(payload)-read) { // #nosec G115 -- comparison stays unsigned.
		return nil, errors.New("invalid length-delimited hand-off")
	}
	return payload[read : read+int(length)], nil // #nosec G115 -- bounded by the payload length above.
}

func testProtobufBytesField(data []byte, wanted uint64) []byte {
	for len(data) > 0 {
		tag, tagBytes := binary.Uvarint(data)
		if tagBytes <= 0 {
			return nil
		}
		data = data[tagBytes:]
		if tag&7 == 0 {
			_, size := binary.Uvarint(data)
			if size <= 0 {
				return nil
			}
			data = data[size:]
			continue
		}
		if tag&7 != 2 {
			return nil
		}
		length, size := binary.Uvarint(data)
		if size <= 0 || length > uint64(len(data)-size) { // #nosec G115 -- comparison stays unsigned.
			return nil
		}
		fieldLength := int(length) // #nosec G115 -- bounded by the remaining slice above.
		value := data[size : size+fieldLength]
		if tag>>3 == wanted {
			return value
		}
		data = data[size+fieldLength:]
	}
	return nil
}
