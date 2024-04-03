package clients

import (
	errors "github.com/rotisserie/eris"
)

const (
	DefaultRootKey = "gloo" // used for vault and consul key-value storage
)

var (
	// ErrNotImplemented indicates a call was made to an interface method which has not been implemented
	ErrNotImplemented = errors.New("interface method not implemented")
)
