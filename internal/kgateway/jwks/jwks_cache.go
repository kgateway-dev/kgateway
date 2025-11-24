package jwks

import (
	"encoding/json"
	"errors"
	"sync"

	"github.com/go-jose/go-jose/v4"
)

type jwksCache struct {
	l    sync.Mutex
	jwks map[string]string // jwks uri -> jwks
}

func NewJwksCache() *jwksCache {
	return &jwksCache{
		jwks: make(map[string]string),
	}
}

// Re-create jwks cache from the state persisted in ConfigMaps.
func (c *jwksCache) LoadJwksFromStores(storedJwks map[string]string) error {
	newCache := NewJwksCache()
	errs := make([]error, 0)

	for uri, serializedJwks := range storedJwks {
		// deserialize jwks to validate it
		jwks := jose.JSONWebKeySet{}
		if err := json.Unmarshal([]byte(serializedJwks), &jwks); err != nil {
			errs = append(errs, err)
			continue
		}
		newCache.compareAndAddJwks(uri, jwks)
	}

	c.l.Lock()
	c.jwks = newCache.jwks
	c.l.Unlock()
	return errors.Join(errs...)
}

func (c *jwksCache) GetJwks(uri string) (string, bool) {
	c.l.Lock()
	defer c.l.Unlock()

	jwks, ok := c.jwks[uri]
	return jwks, ok
}

// Add a jwks to cache. If an exact same jwks is already present in the cache, the result is a nop.
// TODO (dmitri-d) check for max size
func (c *jwksCache) compareAndAddJwks(uri string, jwks jose.JSONWebKeySet) (string, error) {
	serializedJwks, err := json.Marshal(jwks)
	if err != nil {
		return "", err
	}

	c.l.Lock()
	defer c.l.Unlock()

	if j, ok := c.jwks[uri]; ok {
		if j == string(serializedJwks) {
			return "", nil
		}
	}

	c.jwks[uri] = string(serializedJwks)
	return c.jwks[uri], nil
}

// Remove jwks from cache.
func (c *jwksCache) deleteJwks(uri string) {
	c.l.Lock()
	delete(c.jwks, uri)
	c.l.Unlock()
}
