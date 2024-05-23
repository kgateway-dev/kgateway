package iosnapshot

import (
	"fmt"
	"net/http"
	"strings"
)

// Server responds to requests for Input Snapshots, and returns them
type Server interface {
	http.Handler
}

// NewInputServer returns an implementation of a server that returns Input Snapshots
func NewInputServer(history History) Server {
	return &inputServer{
		history: history,
	}
}

type inputServer struct {
	history History
}

func (s *inputServer) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	format, requestFilters := getUrlParams(request)

	writer.Header().Set("Content-Type", getContentType(format))

	b, err := s.history.GetFilteredInput(format, requestFilters)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	_, _ = fmt.Fprint(writer, string(b))
}

func getUrlParams(r *http.Request) (string, Filters) {
	format := getSingleValueQuery(r, "format")
	filters := NewFilters(
		getMultiValueQuery(r, "namespaces"),
	)

	return format, filters
}

func getMultiValueQuery(r *http.Request, key string) []string {
	valueS := r.FormValue(key)
	if valueS != "" {
		return strings.Split(valueS, "::")
	}

	return nil
}

func getSingleValueQuery(r *http.Request, key string) string {
	return r.FormValue(key)
}
