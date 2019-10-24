// AUTOMATICALLY GENERATED FILE. DO NOT EDIT.

// +build !dev

package assets

import (
	"net/http"
	"strings"
	"time"
)

type asset struct {
	Name    string
	Content string
	etag    string
}

func (a asset) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if a.etag != "" && w.Header().Get("ETag") == "" {
		w.Header().Set("ETag", a.etag)
	}
	body := strings.NewReader(a.Content)
	http.ServeContent(w, req, a.Name, time.Time{}, body)
}
