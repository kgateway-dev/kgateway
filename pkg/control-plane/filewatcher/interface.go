package filewatcher

import "github.com/solo-io/gloo/pkg/storage/dependencies"

// map [ref] : *File
type Files map[string]*dependencies.File

// Interface is responsible for watching artifacts referenced by a config
type Interface interface {
	Run(<-chan struct{})

	TrackFiles(fileRefs []string)

	// artifacts are pushed here whenever they are read
	Files() <-chan Files

	// should show valid if the most recent update passed, otherwise a useful error
	Error() <-chan error
}
