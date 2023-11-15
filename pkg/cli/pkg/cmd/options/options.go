package options

import (
	"context"
	"time"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options/contextoptions"
	printTypes "github.com/solo-io/gloo/projects/gloo/cli/pkg/printers"
)

type Options struct {
	Top       Top
	Install   Install
	Uninstall Uninstall
	Check     Check
}
type Top struct {
	contextoptions.ContextAccessible
	CheckName          []string
	Output             printTypes.OutputType
	Namespace          string
	Ctx                context.Context
	Zip                bool
	ResourceNamespaces []string // namespaces in which to check custom resources
}
type Install struct {
	Gloo    HelmInstall
	DryRun  bool
	Version string
}

type HelmInstall struct {
	CreateNamespace         bool
	Namespace               string
	HelmChartOverride       string
	HelmChartValueFileNames []string
	HelmReleaseName         string
}
type HelmUninstall struct {
	Namespace       string
	HelmReleaseName string
	DeleteCrds      bool
	DeleteNamespace bool
	DeleteAll       bool
}

type Uninstall struct {
	GlooUninstall HelmUninstall
}

type Check struct {
	// The maximum length of time alloted to `glooctl check`. A value of zero means no timeout.
	CheckTimeout time.Duration
}
