package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strings"
)

// ServerConfiguration contains only public multiplayer discovery and client
// metadata. Credentials are never serialized with launcher settings.
type ServerConfiguration struct {
	Issuer            string `json:"issuer"`
	ControlAPIBaseURL string `json:"controlApiBaseUrl"`
	BootstrapURL      string `json:"bootstrapUrl"`
	LaunchJWKSURL     string `json:"launchJwksUrl"`
	ProtocolVersion   uint32 `json:"launchProtocolVersion"`
	BuildHash         string `json:"launchBuildHash"`
	LauncherClientID  string `json:"launcherClientId"`
	DeviceClientID    string `json:"deviceClientId"`
	AudienceProjectID string `json:"audienceProjectId,omitempty"`
	Scopes            string `json:"scopes,omitempty"`
}

func loadServerConfiguration(path string) (ServerConfiguration, error) {
	return loadServerConfigurationWithLookup(path, net.LookupIP)
}

func loadServerConfigurationWithLookup(path string, lookup func(string) ([]net.IP, error)) (ServerConfiguration, error) {
	data, err := os.ReadFile(path) // #nosec G304,G703 -- the user explicitly selected this developer configuration.
	if err != nil {
		return ServerConfiguration{}, err
	}
	if len(data) > 64<<10 {
		return ServerConfiguration{}, errors.New("developer server configuration is too large")
	}
	var config ServerConfiguration
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return ServerConfiguration{}, fmt.Errorf("developer server configuration is invalid: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return ServerConfiguration{}, errors.New("developer server configuration contains trailing data")
	}
	return validateServerConfigurationWithLookup(config, lookup)
}

func validateServerConfiguration(config ServerConfiguration) (ServerConfiguration, error) {
	return validateServerConfigurationWithLookup(config, net.LookupIP)
}

func validateServerConfigurationWithLookup(config ServerConfiguration, lookup func(string) ([]net.IP, error)) (ServerConfiguration, error) {
	var err error
	if config.Issuer, err = validateServerURL("issuer", config.Issuer, true, lookup); err != nil {
		return ServerConfiguration{}, err
	}
	if config.ControlAPIBaseURL, err = validateServerURL("control API", config.ControlAPIBaseURL, false, lookup); err != nil {
		return ServerConfiguration{}, err
	}
	if config.BootstrapURL, err = validateServerURL("bootstrap", config.BootstrapURL, false, lookup); err != nil {
		return ServerConfiguration{}, err
	}
	if config.LaunchJWKSURL, err = validateServerURL("launch JWKS", config.LaunchJWKSURL, false, lookup); err != nil {
		return ServerConfiguration{}, err
	}
	config.LauncherClientID = strings.TrimSpace(config.LauncherClientID)
	config.DeviceClientID = strings.TrimSpace(config.DeviceClientID)
	config.AudienceProjectID = strings.TrimSpace(config.AudienceProjectID)
	config.Scopes = strings.Join(strings.Fields(config.Scopes), " ")
	if config.LauncherClientID == "" || config.DeviceClientID == "" {
		return ServerConfiguration{}, errors.New("developer server configuration requires both launcher client IDs")
	}
	buildHash, hashErr := hex.DecodeString(config.BuildHash)
	if config.ProtocolVersion == 0 || hashErr != nil || len(buildHash) != 32 {
		return ServerConfiguration{}, errors.New("developer server configuration has invalid launch metadata")
	}
	config.BuildHash = strings.ToLower(config.BuildHash)
	if config.Scopes == "" {
		config.Scopes = productionScopes(config.AudienceProjectID)
	}
	return config, nil
}

func validateServerURL(label, value string, issuer bool, lookup func(string) ([]net.IP, error)) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("%s URL is invalid", label)
	}
	if issuer && parsed.Path != "" && parsed.Path != "/" {
		return "", errors.New("issuer URL cannot contain a path")
	}
	if parsed.Scheme != "https" {
		if parsed.Scheme != "http" || !loopbackHost(parsed.Hostname(), lookup) {
			return "", fmt.Errorf("%s must use HTTPS unless it resolves only to loopback", label)
		}
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func loopbackHost(host string, lookup func(string) ([]net.IP, error)) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	if address := net.ParseIP(host); address != nil {
		return address.IsLoopback()
	}
	addresses, err := lookup(host)
	if err != nil || len(addresses) == 0 {
		return false
	}
	for _, address := range addresses {
		if !address.IsLoopback() {
			return false
		}
	}
	return true
}

func productionScopes(projectID string) string {
	base := "openid offline_access urn:zitadel:iam:user:resourceowner"
	if projectID == "" {
		return base
	}
	return base + " urn:zitadel:iam:org:project:id:" + projectID + ":aud"
}
