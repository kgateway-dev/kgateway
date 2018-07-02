package configwatcher

import "github.com/solo-io/gloo/pkg/api/types/v1"

type Interface interface {
	Run(stop <-chan struct{})
	Config() <-chan *v1.Config
	Error() <-chan error
}
