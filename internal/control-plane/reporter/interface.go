package reporter

import (
	"github.com/solo-io/gloo/pkg/api/types/v1"
)

type ConfigObjectError struct {
	CfgObject v1.ConfigObject
	Err       error
}

type Interface interface {
	WriteGlobalReports(cfgObjectErrs []ConfigObjectError) error
	WriteRoleReports(role string, cfgObjectErrs []ConfigObjectError) error
}
