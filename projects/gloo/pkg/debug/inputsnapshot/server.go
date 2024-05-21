package inputsnapshot

import (
	"net/http"
)

func init() {
	DefaultServer = NewServer()
}

var (
	DefaultServer Server
)

// Server responds to requests for Input Snapshots, and returns them
type Server interface {
	http.Handler
}

// NewServer returns an implementation of the InputSnapshot server
func NewServer() Server {
	return &server{}
}

type server struct {
}

func (s server) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	//TODO implement me
	panic("implement me")
}
