package reporter

import (
	"github.com/solo-io/gloo/pkg/api/types/v1"
)

type ConfigObjectReport struct {
	CfgObject v1.ConfigObject
	Err       error
}

type Interface interface {
	WriteReports(statuses []ConfigObjectReport) error
}
