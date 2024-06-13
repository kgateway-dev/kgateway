package utils

import (
	"strconv"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

// HashAndStoreHttpOptions will hash the provided httpOptions and store it using the hash as a key in the provided map.
// It will return the hash of the httpOptions as a string
func HashAndStoreHttpOptions(
	httpOptions *gloov1.HttpListenerOptions,
	httpOptionsByName map[string]*gloov1.HttpListenerOptions,
) string {
	// store HttpListenerOptions, indexed by a hash of the httpOptions
	httpOptionsHash, _ := httpOptions.Hash(nil)
	httpOptionsRef := strconv.Itoa(int(httpOptionsHash))
	httpOptionsByName[httpOptionsRef] = httpOptions
	return httpOptionsRef
}
