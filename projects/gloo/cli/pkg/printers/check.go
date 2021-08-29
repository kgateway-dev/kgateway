package printers

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/check"
)
type checkResponse struct {
	Resource *checkStatus
	Errors   error
}
type checkStatus struct {
	Name   string
	Status string
}

PrintCheck(checkResponse response, outputType OutputType) error {
checkRes
}