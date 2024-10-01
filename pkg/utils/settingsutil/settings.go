package settingsutil

import (
	"context"
	"fmt"
	"slices"
	"sync"

	utils_namespaces "github.com/solo-io/gloo/pkg/utils/namespaces"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/lru"
)

type settingsKeyStruct struct{}

var (
	// This mutex ensures that the namespaces to watch are generated and cached synchronously
	// to prevent any issues when translation is running while the settings CR has been updated
	mu sync.Mutex

	// Setting a cache size of 2 should suffice for :
	// - The current translation loop
	// - The new loop about to run when the settings have changed
	namespacesToWatchCache = lru.New(2)

	settingsKey = settingsKeyStruct{}
)

func WithSettings(ctx context.Context, settings *v1.Settings) context.Context {
	return context.WithValue(ctx, settingsKey, settings)
}

// Deprecated: potentially unsafe panic
func FromContext(ctx context.Context) *v1.Settings {
	if ctx == nil {
		return nil
	}
	settings := MaybeFromContext(ctx)
	if settings != nil {
		return settings
	}
	// we should always have settings when this method is called.
	panic("no settings on context")
}

func MaybeFromContext(ctx context.Context) *v1.Settings {
	if ctx == nil {
		return nil
	}
	if settings, ok := ctx.Value(settingsKey).(*v1.Settings); ok {
		return settings
	}
	return nil
}

func IsAllNamespacesFromSettings(settings *v1.Settings) bool {
	if settings == nil {
		return false
	}
	return IsAllNamespaces(GetNamespacesToWatch(settings))
}

func IsAllNamespaces(watchNs []string) bool {
	switch {
	case len(watchNs) == 0:
		return true
	case len(watchNs) == 1 && watchNs[0] == "":
		return true
	default:
		return false
	}
}

// GenerateNamespacesToWatch generates the list of namespaces to watch based on :
// - If `watchNamespaces` is defined, return it and do not consider `watchNamespaceSelectors`
// - If `watchNamespaces` and `watchNamespaceSelectors` are not defined, return `watchNamespaces` for backward compatibility
// - If `watchNamespaces` is not defined and `watchNamespaceSelectors` is defined, return all namespaces that match the `watchNamespaceSelectors`
// In every case, the `discoveryNamespace` (defaults to `gloo-system`) is appended to the list of namespaces
func GenerateNamespacesToWatch(settings *v1.Settings, namespaces kubernetes.KubeNamespaceList) ([]string, error) {
	writeNamespace := generateDiscoveryNamespace(settings)

	if len(settings.GetWatchNamespaces()) != 0 {
		// Prevent an error where the controller can not read resources written by discovery
		// if the install or discovery namespace is not watched
		return utils_namespaces.ProcessWatchNamespaces(settings.GetWatchNamespaces(), writeNamespace), nil
	}

	// If neither `watchNamespaces` nor `watchNamespaceSelectors` is specified, return `watchNamespaces`
	// for backward compatibility. This could either be nil or an empty list.
	if len(settings.GetWatchNamespaceSelectors()) == 0 {
		return settings.GetWatchNamespaces(), nil
	}

	var selectors []labels.Selector
	selectedNamespaces := sets.NewString()

	selectors, err := labelSelectorsAsSelectors(settings.GetWatchNamespaceSelectors())
	if err != nil {
		return nil, err
	}

	for _, ns := range namespaces {
		if namespaceMatchesSelector(*ns, selectors) {
			selectedNamespaces.Insert(ns.Name)
		}
	}

	// Prevent an error where the controller can not read resources written by discovery
	// if the install or discovery namespace is not watched
	// This also doubles as a way to ensure there is at least one namespace watched
	selectedNamespaces.Insert(writeNamespace)

	// This returns the contents as a sorted slice.
	return selectedNamespaces.List(), nil
}

func generateDiscoveryNamespace(settings *v1.Settings) string {
	writeNamespace := settings.GetDiscoveryNamespace()
	if writeNamespace == "" {
		writeNamespace = defaults.GlooSystem
	}
	return writeNamespace
}

// NamespaceWatched returns true if the namespace passed will be watched based on
// the current settings object's `watchNamespaces` and `watchNamespaceSelectors` fields
func NamespaceWatched(settings *v1.Settings, namespace kubernetes.KubeNamespace) (bool, error) {
	namespacesToWatch, err := GenerateNamespacesToWatch(settings, kubernetes.KubeNamespaceList{&namespace})
	if err != nil {
		return false, err
	}
	return slices.Contains(namespacesToWatch, namespace.GetName()), nil
}

func setNamespacesToWatch(settings *v1.Settings, namespaces []string) {
	namespacesToWatchCache.Add(settings.MustHash(), namespaces)
}

// UpdateNamespacesToWatch generates and updated the list of namespaces to watch and returns true if updated
func UpdateNamespacesToWatch(settings *v1.Settings, namespaces kubernetes.KubeNamespaceList) (bool, error) {
	// Run this method synchronously to prevent any issues with caching the namespaces to watch
	// This can occur if the settings object has changed twice and the former settings' context has been cancelled
	// but the methods that rely on the former context have not yet processed its cancellation
	mu.Lock()
	defer mu.Unlock()

	newNamespacesToWatch, err := GenerateNamespacesToWatch(settings, namespaces)
	if err != nil {
		return false, err
	}

	ns, ok := namespacesToWatchCache.Get(settings.MustHash())
	if ok {
		currentNamespacesToWatch, ok := ns.([]string)
		if ok && slices.Equal(newNamespacesToWatch, currentNamespacesToWatch) {
			return false, nil
		}
	}

	setNamespacesToWatch(settings, newNamespacesToWatch)
	return true, nil
}

func getAllNamespaces() (kubernetes.KubeNamespaceList, error) {
	// Create a context and cancel it right after we get the list of namespaces
	// to prevent goroutine leaks
	// Since this method is called only when the cache isn't already populated (from the cli or e2e tests)
	// this should not cause any issues
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	namespaceClient, err := utils_namespaces.NewKubeNamespaceClient(ctx)
	// If there is any error when creating a KubeNamespaceClient (RBAC issues) default to a fake client
	if err != nil {
		namespaceClient = &utils_namespaces.NoOpKubeNamespaceWatcher{}
	}
	return namespaceClient.List(clients.ListOpts{})
}

// GetNamespacesToWatch returns the list of namespaces to watch based on the last run of `GenerateNamespacesToWatch`
func GetNamespacesToWatch(settings *v1.Settings) []string {
	ns, ok := namespacesToWatchCache.Get(settings.MustHash())
	if ok {
		currentNamespacesToWatch, ok := ns.([]string)
		if ok {
			return currentNamespacesToWatch
		}
	}

	writeNamespace := generateDiscoveryNamespace(settings)

	// Short-circuit to avoid creating a client
	if len(settings.GetWatchNamespaces()) != 0 {
		// Prevent an error where the controller can not read resources written by discovery
		// if the install or discovery namespace is not watched
		return utils_namespaces.ProcessWatchNamespaces(settings.GetWatchNamespaces(), writeNamespace)
	}

	// If neither `watchNamespaces` nor `watchNamespaceSelectors` is specified, return `watchNamespaces`
	// for backward compatibility. This could either be nil or an empty list.
	if len(settings.GetWatchNamespaceSelectors()) == 0 {
		return settings.GetWatchNamespaces()
	}

	// Fallback to fetching all namespaces and updating the cache if not found
	allNamespaces, err := getAllNamespaces()
	if err != nil {
		fmt.Println("Unable to fetch namespaces")
	}
	UpdateNamespacesToWatch(settings, allNamespaces)
	ns, _ = namespacesToWatchCache.Get(settings.MustHash())
	return ns.([]string)
}

func namespaceMatchesSelector(ns kubernetes.KubeNamespace, selectors []labels.Selector) bool {
	for _, selector := range selectors {
		if selector.Matches(labels.Set(ns.GetLabels())) {
			return true
		}
	}
	return false
}

func labelSelectorsAsSelectors(labelSelectors []*v1.LabelSelector) ([]labels.Selector, error) {
	var selectors []labels.Selector
	for _, selector := range labelSelectors {
		ls, err := labelSelectorAsSelector(selector)
		if err != nil {
			return nil, err
		}
		selectors = append(selectors, ls)
	}
	return selectors, nil
}

// Copied over from https://github.com/istio/istio/blob/cef09ad2a487d829d36a72779bd936412dd82102/pkg/kube/namespace/filter.go#L152-L190
func labelSelectorAsSelector(labelSelectors *v1.LabelSelector) (labels.Selector, error) {
	if labelSelectors == nil {
		return labels.Nothing(), nil
	}
	if len(labelSelectors.GetMatchLabels())+len(labelSelectors.GetMatchExpressions()) == 0 {
		return labels.Everything(), nil
	}
	requirements := make([]labels.Requirement, 0, len(labelSelectors.GetMatchLabels())+len(labelSelectors.GetMatchExpressions()))
	for k, v := range labelSelectors.GetMatchLabels() {
		r, err := labels.NewRequirement(k, selection.Equals, []string{v})
		if err != nil {
			return nil, err
		}
		requirements = append(requirements, *r)
	}
	for _, expr := range labelSelectors.GetMatchExpressions() {
		var op selection.Operator
		switch metav1.LabelSelectorOperator(expr.GetOperator()) {
		case metav1.LabelSelectorOpIn:
			op = selection.In
		case metav1.LabelSelectorOpNotIn:
			op = selection.NotIn
		case metav1.LabelSelectorOpExists:
			op = selection.Exists
		case metav1.LabelSelectorOpDoesNotExist:
			op = selection.DoesNotExist
		default:
			return nil, fmt.Errorf("%q is not a valid label selector operator", expr.GetOperator())
		}
		r, err := labels.NewRequirement(expr.GetKey(), op, append([]string(nil), expr.GetValues()...))
		if err != nil {
			return nil, err
		}
		requirements = append(requirements, *r)
	}
	selector := labels.NewSelector()
	selector = selector.Add(requirements...)
	return selector, nil
}
