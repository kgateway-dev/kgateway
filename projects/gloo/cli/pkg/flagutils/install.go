package flagutils

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/spf13/pflag"
)

func AddInstallFlags(set *pflag.FlagSet, install *options.Install) {
	addSecretFlags(set, install)
	set.BoolVarP(&install.DryRun, "dry-run", "d", false, "Dump the raw installation yaml instead of applying it to kubernetes")
	set.StringVar(&install.ReleaseVersion, "release", "", "install using this release version. defaults to the latest github release")
	set.StringVarP(&install.GlooManifestOverride, "file", "f", "", "Install Gloo from this kubernetes manifest yaml file rather than from a release")
}

func AddKnativeInstallFlags(set *pflag.FlagSet, knative *options.KnativeInstall) {
	set.StringVar(&knative.CrdManifestOverride, "knative-crds-manifest", "", "Install Knative CRDs from this kubernetes manifest yaml file rather than from a release")
	set.StringVar(&knative.InstallManifestOverride, "knative-install-manifest", "", "Install Knative Serving from this kubernetes manifest yaml file rather than from a release")
}

func addSecretFlags(set *pflag.FlagSet, install *options.Install) {
	set.StringVar(&install.DockerAuth.Email, "docker-email", "", "Email for docker registry. Use for pulling private images.")
	set.StringVar(&install.DockerAuth.Username, "docker-username", "", "Username for Docker registry authentication. Use for pulling private images.")
	set.StringVar(&install.DockerAuth.Password, "docker-password", "", "Password for docker registry authentication. Use for pulling private images.")
	set.StringVar(&install.DockerAuth.Server, "docker-server", "https://index.docker.io/v1/", "Docker server to use for pulling images")
}
