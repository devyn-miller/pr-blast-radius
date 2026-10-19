// go-core/internal/cache/cached_registry.go
package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/devyn-miller/pr-blast-radius/go-core/internal/astparse"
)

// CachedRegistry decorates an *astparse.Registry with content-hash-keyed
// caching, satisfying the same shape BuildGraph already expects
// (a ParseFile method) — this is the Decorator pattern, chosen specifically
// so BuildGraph's existing tests never needed to change to support caching.
type CachedRegistry struct {
	inner *astparse.Registry
	store Store
}

func NewCachedRegistry(inner *astparse.Registry, store Store) *CachedRegistry {
	return &CachedRegistry{inner: inner, store: store}
}

func (c *CachedRegistry) ParseFile(ctx context.Context, filePath string, source []byte) (astparse.FileCaptures, error) {
	sha := contentHash(source)

	if cached, hit, err := c.store.GetFileCaptures(filePath, sha); err == nil && hit {
		return *cached, nil
	}

	fc, err := c.inner.ParseFile(ctx, filePath, source)
	if err != nil {
		return astparse.FileCaptures{}, err
	}

	_ = c.store.PutFileCaptures(filePath, sha, fc) // cache-write failure is non-fatal — degrades to "always cold" gracefully
	return fc, nil
}

func contentHash(source []byte) string {
	sum := sha256.Sum256(source)
	return hex.EncodeToString(sum[:])
}
