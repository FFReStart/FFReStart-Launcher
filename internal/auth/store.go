package auth

import (
	"context"
	"errors"
	"sync"

	keyring "github.com/zalando/go-keyring"
)

const KeyringService = "FFReStart Launcher"

type RefreshStore interface {
	Save(string) error
	Load() (string, error)
	Clear() error
}

type MemoryStore struct {
	mu    sync.Mutex
	token string
}

func (m *MemoryStore) Save(value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.token = value
	return nil
}
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

func (k KeyringStore) Save(value string) error { return keyring.Set(k.Service, k.User, value) }
func (k KeyringStore) Load() (string, error)   { return keyring.Get(k.Service, k.User) }
func (k KeyringStore) Clear() error {
	err := keyring.Delete(k.Service, k.User)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}

// FallbackStore uses the OS keyring when possible and process memory otherwise.
type FallbackStore struct {
	Primary RefreshStore
	Memory  *MemoryStore
}

func (f *FallbackStore) Save(value string) error {
	if err := f.Primary.Save(value); err == nil {
		_ = f.Memory.Clear()
		return nil
	}
	return f.Memory.Save(value)
}
func (f *FallbackStore) Load() (string, error) {
	if value, err := f.Primary.Load(); err == nil {
		return value, nil
	}
	return f.Memory.Load()
}
func (f *FallbackStore) Clear() error                                      { return errors.Join(f.Primary.Clear(), f.Memory.Clear()) }
func (f *FallbackStore) LoadRefresh(context.Context) (string, error)       { return f.Load() }
func (f *FallbackStore) SaveRefresh(_ context.Context, value string) error { return f.Save(value) }
