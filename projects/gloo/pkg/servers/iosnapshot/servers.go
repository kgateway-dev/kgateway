package iosnapshot

import (
	"fmt"
	"net/http"
)

// NewInputServer returns an implementation of a server that returns Input Snapshots
func NewInputServer(history History) http.Handler {
	return &inputServer{
		history: history,
	}
}

type inputServer struct {
	history History
}

func (s *inputServer) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	format := request.FormValue("format")

	writer.Header().Set("Content-Type", getContentType(format))

	b, err := s.history.GetInput()
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	_, _ = fmt.Fprint(writer, string(b))
}
