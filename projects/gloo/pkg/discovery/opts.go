package discovery

import v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"

type Opts struct {
	KubeOpts struct {
		IgnoredServices []string
	}
	Settings *v1.Settings
}
