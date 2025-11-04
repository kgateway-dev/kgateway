package krtequals

import (
	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

func init() {
	register.Plugin("krtequals", New)
}

type plugin struct {
	cfg Config
}

var _ register.LinterPlugin = &plugin{}

// New constructs the module plugin for golangci-lint.
func New(settings any) (register.LinterPlugin, error) {
	s, err := register.DecodeSettings[Config](settings)
	if err != nil {
		return nil, err
	}
	return &plugin{cfg: s}, nil
}

func (p *plugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{newAnalyzer(&p.cfg)}, nil
}

func (p *plugin) GetLoadMode() string {
	return register.LoadModeTypesInfo
}
