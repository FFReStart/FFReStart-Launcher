package auth

import (
	"errors"
	"sync"

	keyring "github.com/zalando/go-keyring"
)

const KeyringService = "FFReStart-wails-spike-test"

type RefreshStore interface {
	Save(string) error
	Load() (string, error)
	Clear() error
}

type MemoryStore struct {
	mu    sync.Mutex
	token string
}

func (m *MemoryStore) Save(v string) error { m.mu.Lock(); defer m.mu.Unlock(); m.token = v; return nil }
func (m *MemoryStore) Load() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.token == "" {
		return "", keyring.ErrNotFound
	}
	return m.token, nil
}
func (m *MemoryStore) Clear() error { m.mu.Lock(); defer m.mu.Unlock(); m.token = ""; return nil }

type KeyringStore struct{ Service, User string }

func (k KeyringStore) Save(v string) error   { return keyring.Set(k.Service, k.User, v) }
func (k KeyringStore) Load() (string, error) { return keyring.Get(k.Service, k.User) }
func (k KeyringStore) Clear() error {
	err := keyring.Delete(k.Service, k.User)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}

// FallbackStore never persists outside the OS keychain. When it is unavailable,
// the refresh token survives only for the lifetime of this process.
type FallbackStore struct {
	Primary RefreshStore
	Memory  *MemoryStore
}

func (f *FallbackStore) Save(v string) error {
	if err := f.Primary.Save(v); err == nil {
		_ = f.Memory.Clear()
		return nil
	}
	return f.Memory.Save(v)
}
func (f *FallbackStore) Load() (string, error) {
	if v, err := f.Primary.Load(); err == nil {
		return v, nil
	}
	return f.Memory.Load()
}
func (f *FallbackStore) Clear() error { _ = f.Primary.Clear(); return f.Memory.Clear() }
