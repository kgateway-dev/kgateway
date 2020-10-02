package helm

import (
	"github.com/rotisserie/eris"
	"helm.sh/helm/v3/pkg/release"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"sigs.k8s.io/yaml"
)

// Some resources can be duplicated because of weirdness with Helm hooks, however we have a test that
// makes sure we don't produce such charts anymore, so this just returns all resources now.
func GetHooks(hooks []*release.Hook) (results []*release.Hook, err error) {
	for _, hook := range hooks {
		// Parse the resource in order to access the annotations
		var resource struct{ Metadata v1.ObjectMeta }
		if err := yaml.Unmarshal([]byte(hook.Manifest), &resource); err != nil {
			return nil, eris.Wrapf(err, "parsing resource: %s", hook.Manifest)
		}
		results = append(results, hook)
	}

	return results, nil
}
