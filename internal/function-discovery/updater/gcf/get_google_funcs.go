package gcf

import (
	"context"
	"fmt"
	"net/http"
	"unicode/utf8"

	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/cloudfunctions/v1beta2"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	googleplugin "github.com/solo-io/gloo/pkg/plugins/google"
	"github.com/solo-io/gloo/pkg/secretwatcher"
)

var missingAnnotationErr = errors.Errorf("Google Function Discovery requires that a secret ref for a secret containing "+
"Google Cloud account credentials be specified in the annotations for each Google Cloud Upstream. "+
"The annotation key is %v. The annotations should contain the annotation %v: [your_secret_ref]",
annotationKey, annotationKey)

const (
	// expected annotation key for secret ref
	// TODO: make sure this is well documented
	annotationKey = "gloo.solo.io/google_secret_ref"

	// expected map identifiers for secrets
	serviceAccountJsonKeyFile = "json_key_file"

	// v1beta2: https://cloud.google.com/functions/docs/reference/rest/v1beta2/projects.locations.functions
	statusReady = "READY"
	// v1 status: https://cloud.google.com/functions/docs/reference/rest/v1/projects.locations.functions
	statusActive = "ACTIVE"
)

func GetFuncs(us *v1.Upstream, secrets secretwatcher.SecretMap) ([]*v1.Function, error) {

	secretRef, err := GetSecretRef(us)
	if err != nil {
		return nil, errors.Wrap(err, "getting secret ref")
	}
	googleSpec, err := googleplugin.DecodeUpstreamSpec(us.Spec)
	if err != nil {
		return nil, errors.Wrap(err, "decoding gcf upstream spec")
	}
	googleSecrets, ok := secrets[secretRef]
	if !ok {
		return nil, errors.Wrapf(err, "secrets not found for secret ref %v", secretRef)
	}

	jsonKey, ok := googleSecrets[serviceAccountJsonKeyFile]
	if !ok {
		return nil, errors.Errorf("key %v missing from provided secret", serviceAccountJsonKeyFile)
	}
	if !utf8.Valid([]byte(jsonKey)) {
		return nil, errors.Errorf("%s not a valid string", serviceAccountJsonKeyFile)
	}

	ctx := context.Background()

	client, err := newGoogleClient(ctx, jsonKey)
	if err != nil {
		return nil, errors.Wrap(err, "creating google oauth2 client")
	}

	gcf, err := cloudfunctions.New(client)
	if err != nil {
		return nil, errors.Wrap(err, "creating gcf client")
	}

	locationID := "-" // all locations
	parent := fmt.Sprintf("projects/%s/locations/%s", googleSpec.ProjectId, locationID)
	listCall := gcf.Projects.Locations.Functions.List(parent)
	var results []*cloudfunctions.CloudFunction
	if err := listCall.Pages(ctx, func(response *cloudfunctions.ListFunctionsResponse) error {
		for _, result := range response.Functions {
			// TODO: document that we currently only support https trigger funcs
			if ((result.Status == statusReady) || (result.Status == statusActive)) && result.HttpsTrigger != nil {
				results = append(results, result)
			}
		}
		return nil
	}); err != nil {
		return nil, errors.Wrap(err, "unable to get list of GCF functions")
	}

	return convertGfuncsToFunctionSpec(results), nil
}

func GetSecretRef(us *v1.Upstream) (string, error) {
	if us.Metadata == nil {
		return "", missingAnnotationErr
	}
	secretRef, ok := us.Metadata.Annotations[annotationKey]
	if !ok {
		return "", missingAnnotationErr
	}
	return secretRef, nil
}

func convertGfuncsToFunctionSpec(results []*cloudfunctions.CloudFunction) []*v1.Function {
	var funcs []*v1.Function
	for _, gFunc := range results {
		fn := &v1.Function{
			Name: gFunc.Name,
			Spec: googleplugin.EncodeFunctionSpec(googleplugin.FunctionSpec{
				URL: gFunc.HttpsTrigger.Url,
			}),
		}
		funcs = append(funcs, fn)
	}
	return funcs
}

func newGoogleClient(ctx context.Context, jsonKey string) (*http.Client, error) {
	jwtConfig, err := google.JWTConfigFromJSON([]byte(jsonKey), cloudfunctions.CloudPlatformScope)
	if err != nil {
		return nil, errors.Wrap(err, "creating jwt config from service account JSON key file ")
	}
	return oauth2.NewClient(ctx, jwtConfig.TokenSource(ctx)), nil
}
