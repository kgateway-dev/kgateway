package jwks

import (
	"encoding/json"
	"errors"

	"github.com/go-jose/go-jose/v4"
)

type jwksCache struct {
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

	c.jwks = newCache.jwks
	return errors.Join(errs...)
}

// Add a jwks to cache. If an exact same jwks is already present in the cache, the result is a nop.
// TODO (dmitri-d) check for max size
func (c *jwksCache) compareAndAddJwks(uri string, jwks jose.JSONWebKeySet) (string, error) {
	serializedJwks, err := json.Marshal(jwks)
	if err != nil {
		return "", err
	}

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
	delete(c.jwks, uri)
}
