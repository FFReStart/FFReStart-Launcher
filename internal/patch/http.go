package patch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

const defaultManifestLimit = 1 << 20

// FetchAndInstall retrieves one bounded manifest and installs its verified files.
func (i Installer) FetchAndInstall(ctx context.Context, client *http.Client, manifestURL string) error {
	parsedURL, err := url.ParseRequestURI(manifestURL)
	if err != nil || !allowedRemoteURL(parsedURL, !i.Release) {
		return ErrInvalidManifest
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, manifestURL, nil)
	if err != nil {
		return err
	}
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("game manifest HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, defaultManifestLimit+1))
	if err != nil {
		return err
	}
	if len(data) > defaultManifestLimit {
		return errors.New("game manifest exceeds size limit")
	}
	return i.Install(ctx, data)
}
