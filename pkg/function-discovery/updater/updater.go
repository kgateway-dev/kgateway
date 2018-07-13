package updater

import (
	"sort"

	"reflect"

	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/backoff"
	"github.com/solo-io/gloo/pkg/function-discovery/detector"
	"github.com/solo-io/gloo/pkg/function-discovery/functiontypes"
	"github.com/solo-io/gloo/pkg/function-discovery/resolver"
	"github.com/solo-io/gloo/pkg/function-discovery/updater/fission"
	"github.com/solo-io/gloo/pkg/function-discovery/updater/gcf"
	"github.com/solo-io/gloo/pkg/function-discovery/updater/grpc"
	"github.com/solo-io/gloo/pkg/function-discovery/updater/lambda"
	"github.com/solo-io/gloo/pkg/function-discovery/updater/openfaas"
	"github.com/solo-io/gloo/pkg/function-discovery/updater/projectfn"
	"github.com/solo-io/gloo/pkg/function-discovery/updater/swagger"

	"github.com/solo-io/gloo/pkg/function-discovery/updater/azure"
	"github.com/solo-io/gloo/pkg/log"
	azureplugin "github.com/solo-io/gloo/pkg/plugins/azure"
	"github.com/solo-io/gloo/pkg/secretwatcher"
	"github.com/solo-io/gloo/pkg/storage"
	"github.com/solo-io/gloo/pkg/storage/dependencies"
)

func GetSecretRefsToWatch(upstreams []*v1.Upstream) []string {
	var refs []string
	for _, us := range upstreams {
		switch functiontypes.GetFunctionType(us) {
		case functiontypes.FunctionTypeLambda:
			ref, err := lambda.GetSecretRef(us)
			if err != nil {
				continue
			}
			refs = append(refs, ref)
		case functiontypes.FunctionTypeGfuncs:
			ref, err := gcf.GetSecretRef(us)
			if err != nil {
				continue
			}
			refs = append(refs, ref)
		case functiontypes.FunctionTypeAzure:
			ref, err := azure.GetSecretRef(us)
			if err != nil {
				continue
			}
			refs = append(refs, ref)
		}
	}
	return refs
}

// if forceSync is set, ignore the local cache and poll for new function list anyway
// we want to forceSync on every refreshDuration
// on a config / secrets change, we don't want to force sync
// else we can get into an update loop
func UpdateFunctions(resolve resolver.Resolver, gloo storage.Interface, secretStore dependencies.SecretStorage,
	files dependencies.FileStorage,
	upstreamName string, secrets secretwatcher.SecretMap) error {
	us, err := gloo.V1().Upstreams().Get(upstreamName)
	if err != nil {
		return errors.Wrapf(err, "failed to get existing upstream with name %v", upstreamName)
	}

	var funcs []*v1.Function
	switch functiontypes.GetFunctionType(us) {
	case functiontypes.FunctionTypeLambda:
		if len(secrets) == 0 {
			log.Warnf("lambda upstream detected, but no secrets have been read yet")
			return nil
		}
		funcs, err = lambda.GetFuncs(us, secrets)
		if err != nil {
			return errors.Wrap(err, "retrieving lambda functions")
		}
	case functiontypes.FunctionTypeGfuncs:
		if len(secrets) == 0 {
			log.Warnf("google functions upstream detected, but no secrets have been read yet")
			return nil
		}
		funcs, err = gcf.GetFuncs(us, secrets)
		if err != nil {
			return errors.Wrap(err, "retrieving google functions")
		}
	case functiontypes.FunctionTypeSwagger:
		funcs, err = swagger.GetFuncs(us)
		if err != nil {
			return errors.Wrap(err, "retrieving swagger functions")
		}
	case functiontypes.FunctionTypeGRPC:
		funcs, err = grpc.GetFuncs(files, us)
		if err != nil {
			return errors.Wrap(err, "retrieving grpc functions")
		}
	case functiontypes.FunctionTypeOpenFaaS:
		funcs, err = openfaas.GetFuncs(resolve, us)
		if err != nil {
			return errors.Wrap(err, "retreving faas functions")
		}
	case functiontypes.FunctionTypeFission:
		funcs, err = fission.GetFuncs(resolve, us)
		if err != nil {
			return errors.Wrap(err, "retreving fission functions")
		}
	case functiontypes.FunctionTypeProjectFn:
		funcs, err = projectfn.GetFuncs(resolve, us)
		if err != nil {
			return errors.Wrap(err, "retreving projectfn functions")
		}
	case functiontypes.FunctionTypeAzure:
		funcs, secret, err := azure.GetFuncsAndSecret(us, secrets)
		if err != nil {
			return errors.Wrap(err, "retreving azure functions")
		}
		// special case because we need to update azure with the
		// discovered secrets & secret ref
		// TODO(ilackarms): implement an interface that handles azure with more elegance
		return updateAzureUpstream(gloo, secretStore, us.Name, funcs, secret)
	default:
		return nil //errors.Errorf("unknown function type")
	}

	if err := updateUpstreamWithFuncs(gloo, us.Name, funcs); err != nil {
		return errors.Wrap(err, "updating upstream object with new funcs")
	}
	return nil
}

func updateAzureUpstream(gloo storage.Interface, secretStore dependencies.SecretStorage,
	upstreamName string, funcs []*v1.Function, azureSecret *dependencies.Secret) error {
	usToUpdate, err := gloo.V1().Upstreams().Get(upstreamName)
	if err != nil {
		return errors.Wrapf(err, "failed to get existing upstream with name %v", upstreamName)
	}

	azureSpec, err := azureplugin.DecodeUpstreamSpec(usToUpdate.Spec)
	if err != nil {
		return errors.Wrap(err, "decoding azure spec")
	}

	// only write the secret if it doesn't exist, or if the data does not match
	existingSecret, err := secretStore.Get(azureSecret.Ref)
	if err != nil {
		// create secret, it doesn't exist
		if _, err := secretStore.Create(azureSecret); err != nil {
			return errors.Wrap(err, "writing azure secret to storage")
		}
	} else if reflect.DeepEqual(existingSecret.Data, azureSecret.Data) {
		// seret exists but has changed
		if _, err := secretStore.Update(azureSecret); err != nil {
			return errors.Wrap(err, "writing azure secret to storage")
		}
	}

	// only update the upstream spec if the ref doesn't match
	if azureSpec.SecretRef != azureSecret.Ref {
		azureSpec.SecretRef = azureSecret.Ref
		usToUpdate.Spec = azureplugin.EncodeUpstreamSpec(*azureSpec)
		_, err = gloo.V1().Upstreams().Update(usToUpdate)
		if err != nil {
			return err
		}
	}

	return updateUpstreamWithFuncs(gloo, upstreamName, funcs)
}

func updateUpstreamWithFuncs(gloo storage.Interface, upstreamName string, funcs []*v1.Function) error {
	// sort funcs for idempotency
	sort.SliceStable(funcs, func(i, j int) bool {
		return funcs[i].Name < funcs[j].Name
	})

	usToUpdate, err := gloo.V1().Upstreams().Get(upstreamName)
	if err != nil {
		return errors.Wrapf(err, "failed to get existing upstream with name %v", upstreamName)
	}

	// no update to do
	if functionListsEqual(usToUpdate.Functions, funcs) {
		return nil
	}

	usToUpdate.Functions = mergeFuncs(usToUpdate.Functions, funcs)

	_, err = gloo.V1().Upstreams().Update(usToUpdate)
	if err != nil {
		return err
	}
	return nil
}

// get the unique set of funcs between two lists
// if conflict, new wins
func mergeFuncs(oldFuncs, newFuncs []*v1.Function) []*v1.Function {
	var notReplaced []*v1.Function
	for _, oldFunc := range oldFuncs {
		var replace bool
		for _, newFunc := range newFuncs {
			if newFunc.Name == oldFunc.Name {
				replace = true
				break
			}
		}
		if replace {
			continue
		}
		notReplaced = append(notReplaced, oldFunc)
	}
	return append(notReplaced, newFuncs...)
}

func functionListsEqual(funcs1, funcs2 []*v1.Function) bool {
	if len(funcs1) != len(funcs2) {
		return false
	}
	for i := range funcs1 {
		fn1 := funcs1[i]
		fn2 := funcs2[i]
		if !fn1.Equal(fn2) {
			return false
		}
	}
	return true
}

// update the upstream with service info and annotations
func UpdateServiceInfo(gloo storage.Interface,
	upstreamName string,
	marker *detector.Marker) error {

	log.Debugf("attempting to apply update for upstream %v", upstreamName)

	us, err := gloo.V1().Upstreams().Get(upstreamName)
	if err != nil {
		return errors.Wrapf(err, "failed to get existing upstream with name %v", upstreamName)
	}

	svcInfo, annotations, err := marker.DetectFunctionalServiceType(us)
	if err != nil {
		return errors.Wrapf(err, "failed to discover whether %v is a functional upstream", upstreamName)
	}

	// not a functional service type
	if svcInfo == nil && annotations == nil {
		return nil
	}

	return backoff.WithBackoff(func() error {
		usToUpdate, err := gloo.V1().Upstreams().Get(upstreamName)
		if err != nil {
			return errors.Wrapf(err, "failed to get existing upstream with name %v", upstreamName)
		}

		// no update to do
		if svcInfoEqual(usToUpdate, svcInfo) && containsAnnotations(usToUpdate, annotations) {
			return nil
		}

		usToUpdate.Metadata.Annotations = mergeAnnotations(usToUpdate.Metadata.Annotations, annotations)
		usToUpdate.ServiceInfo = svcInfo

		if _, err := gloo.V1().Upstreams().Update(usToUpdate); err != nil {
			return errors.Wrapf(err, "updating upstream %s with service info", upstreamName)
		}
		log.Printf("updated upstream %v", usToUpdate)
		return nil
	}, make(chan struct{}))
}

// get the unique set of funcs between two lists
// if conflict, new wins
func mergeAnnotations(oldAnnotations, newAnnotations map[string]string) map[string]string {
	merged := make(map[string]string)
	for k, v := range oldAnnotations {
		merged[k] = v
	}
	for k, v := range newAnnotations {
		merged[k] = v
	}
	return merged
}

func containsAnnotations(us *v1.Upstream, annotations map[string]string) bool {
	if us.Metadata == nil || us.Metadata.Annotations == nil {
		if len(annotations) == 0 {
			return true
		}
		return false
	}
	for k, v := range annotations {
		if us.Metadata.Annotations[k] != v {
			return false
		}
	}
	return true
}

func svcInfoEqual(us *v1.Upstream, svcInfo *v1.ServiceInfo) bool {
	return reflect.DeepEqual(us.ServiceInfo, svcInfo)
}
