// AUTOMATICALLY GENERATED FILE. DO NOT EDIT.

// +build dev

package assets

import (
	"go/build"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type asset struct {
	Name    string
	Content string
	etag    string
}

func (a asset) init() asset {
	return a
}

func (a asset) importPath() string {
	// filled at code gen time
	return "github.com/pmezard/licenses/assets"
}

func (a asset) Open() (*os.File, error) {
	path := a.importPath()
	pkg, err := build.Import(path, ".", build.FindOnly)
	if err != nil {
		return nil, err
	}
	p := filepath.Join(pkg.Dir, a.Name)
	return os.Open(p)
}

func (a asset) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	body, err := a.Open()
	if err != nil {
		// show the os.Open message, with paths and all, but this only
		// happens in dev mode.
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer body.Close()
	http.ServeContent(w, req, a.Name, time.Time{}, body)
}
