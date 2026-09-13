// Package auth owns optional multiplayer credentials. Tokens remain in Go;
// token-bearing operations must never be bound to Wails.
package auth

import "context"

// TokenVault is the backend-only credential capability reserved for WP8. It is
// never exposed as a Wails binding and the offline path must never call it.
type TokenVault interface {
	LoadRefresh(context.Context) (string, error)
	SaveRefresh(context.Context, string) error
}
