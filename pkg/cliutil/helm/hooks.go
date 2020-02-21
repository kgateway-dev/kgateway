package helm

import (
	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"helm.sh/helm/v3/pkg/release"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"sigs.k8s.io/yaml"
)

// Some resources are duplicated because of weirdness with Helm hooks.
// A job needs a service account/rbac resources, and we would like those to be cleaned up after the job is complete
// this isn't really expressible cleanly through Helm hooks.
func GetNonCleanupHooks(hooks []*release.Hook) (results []*release.Hook, err error) {
	for _, hook := range hooks {
		// Parse the resource in order to access the annotations
		var resource struct{ Metadata v1.ObjectMeta }
		if err := yaml.Unmarshal([]byte(hook.Manifest), &resource); err != nil {
			return nil, eris.Wrapf(err, "parsing resource: %s", hook.Manifest)
		}

		// Skip hook cleanup resources
		if annotations := resource.Metadata.Annotations; len(annotations) > 0 {
			if _, ok := annotations[constants.HookCleanupResourceAnnotation]; ok {
				continue
			}
		}

		results = append(results, hook)
	}

	return results, nil
}
