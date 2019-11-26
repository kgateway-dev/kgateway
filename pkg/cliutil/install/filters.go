package install

import (
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	//"k8s.io/helm/pkg/manifest"
)

// This type represents a function that can be used to filter and transform a list of manifests.
// It returns three values:
//   - skip: if true, the input manifest will be excluded from the output
//   - content: if skip is false, this value will be included in the output manifest
//   - err: if != nil, the whole manifest retrieval operation will fail
//type ManifestFilterFunc func(input []manifest.Manifest) (output []manifest.Manifest, err error)

// We need to define this ourselves, because if we unmarshal into `apiextensions.CustomResourceDefinition`
// we don't get the ObjectMeta (in the yaml they are nested under `metadata`, but the k8s struct has
// them as top level fields...)
type ResourceType struct {
	Metadata v1.ObjectMeta
	v1.TypeMeta
}

// Returns only non-empty manifests
//var ExcludeEmptyManifests ManifestFilterFunc = func(input []manifest.Manifest) ([]manifest.Manifest, error) {
//	var output []manifest.Manifest
//	for _, manifest := range input {
//		if !IsEmptyManifest(manifest.Content) {
//			output = append(output, manifest)
//		}
//
//	}
//	return output, nil
//}
//
//type ResourceMatcherFunc func(resource ResourceType) (bool, error)
//
//var preInstallMatcher ResourceMatcherFunc = func(resource ResourceType) (bool, error) {
//	helmPreInstallHook, ok := resource.Metadata.Annotations[hooks.HookAnno]
//	if !ok || helmPreInstallHook != hooks.PreInstall {
//		return false, nil
//	}
//	return true, nil
//}
//
//var crdInstallMatcher ResourceMatcherFunc = func(resource ResourceType) (bool, error) {
//	crdKind := resource.TypeMeta.Kind == CrdKindName
//	if crdKind {
//		// Check whether the CRD is a Helm "crd-install" hook.
//		// If not, throw an error, because this will cause race conditions when installing with Helm (which is
//		// not the case here, but we want to validate the manifests whenever we have the chance)
//		helmCrdInstallHookAnnotation, ok := resource.Metadata.Annotations[hooks.HookAnno]
//		if !ok || helmCrdInstallHookAnnotation != hooks.CRDInstall {
//			return crdKind, errors.Errorf("CRD [%s] must be annotated as a Helm '%s' hook", resource.Metadata.Name, hooks.CRDInstall)
//		}
//	}
//	return crdKind, nil
//}
//
//var nonCrdInstallMatcher ResourceMatcherFunc = func(resource ResourceType) (bool, error) {
//	isCrdInstall, err := crdInstallMatcher(resource)
//	return !isCrdInstall, err
//}
//
//var nonPreInstallMatcher ResourceMatcherFunc = func(resource ResourceType) (bool, error) {
//	isPreInstall, err := preInstallMatcher(resource)
//	return !isPreInstall, err
//}
//
//var excludeByMatcher = func(input []manifest.Manifest, matches ResourceMatcherFunc) (output []manifest.Manifest, allResourceNames []string, err error) {
//	for _, man := range input {
//		content, resourceNames, err := excludeManifestContentByMatcher(man.Content, matches)
//		if err != nil {
//			return nil, nil, err
//		}
//		allResourceNames = append(allResourceNames, resourceNames...)
//		output = append(output, manifest.Manifest{
//			Name:    man.Name,
//			Head:    man.Head,
//			Content: content,
//		})
//	}
//	return
//}
//
//var excludeManifestContentByMatcher = func(input string, matches ResourceMatcherFunc) (output string, resourceNames []string, err error) {
//	var nonMatching []string
//	for _, doc := range strings.Split(input, "---") {
//		if strings.TrimSpace(doc) == "" {
//			continue
//		}
//
//		var resource ResourceType
//		if err := yaml.Unmarshal([]byte(doc), &resource); err != nil {
//			return "", nil, errors.Wrapf(err, "parsing resource: %s", doc)
//		}
//
//		isMatch, err := matches(resource)
//		if err != nil {
//			return "", nil, err
//		}
//		if !isMatch {
//			resourceNames = append(resourceNames, resource.Metadata.Name)
//			nonMatching = append(nonMatching, doc)
//		}
//	}
//	output = strings.Join(nonMatching, YamlDocumentSeparator)
//	return
//}
//
//var ExcludeMatchingResources = func(matcherFunc ResourceMatcherFunc) ManifestFilterFunc {
//	if matcherFunc == nil {
//		return IdentityFilterFunc
//	}
//	return func(input []manifest.Manifest) (output []manifest.Manifest, err error) {
//		manifest, _, err := excludeByMatcher(input, matcherFunc)
//		return manifest, err
//	}
//}
//
//var IdentityFilterFunc ManifestFilterFunc = func(input []manifest.Manifest) (output []manifest.Manifest, err error) {
//	output = input
//	return
//}
//
//// Filters out any pre-install from each manifest
//var ExcludePreInstall ManifestFilterFunc = func(input []manifest.Manifest) (output []manifest.Manifest, err error) {
//	manifest, _, err := excludeByMatcher(input, preInstallMatcher)
//	return manifest, err
//}
//
//// Filters out anything but pre-install
//var IncludeOnlyPreInstall ManifestFilterFunc = func(input []manifest.Manifest) (output []manifest.Manifest, err error) {
//	manifest, _, err := excludeByMatcher(input, nonPreInstallMatcher)
//	return manifest, err
//}
//
//// Filters out any CRD from each manifest
//var ExcludeCrds ManifestFilterFunc = func(input []manifest.Manifest) (output []manifest.Manifest, err error) {
//	manifest, _, err := excludeByMatcher(input, crdInstallMatcher)
//	return manifest, err
//}
//
//var ExcludeNonCrds = func(input []manifest.Manifest) (output []manifest.Manifest, names []string, err error) {
//	return excludeByMatcher(input, nonCrdInstallMatcher)
//}
//
//// Filters out NOTES.txt files
//var ExcludeNotes ManifestFilterFunc = func(input []manifest.Manifest) (output []manifest.Manifest, err error) {
//	for _, man := range input {
//		if strings.HasSuffix(man.Name, NotesFileName) {
//			continue
//		}
//		output = append(output, man)
//	}
//	return
//}
//
//func KnativeResourceFilterFunction(skipKnative bool) ManifestFilterFunc {
//	return func(input []manifest.Manifest) ([]manifest.Manifest, error) {
//		var output []manifest.Manifest
//		for _, man := range input {
//			if strings.Contains(man.Name, "knative") && skipKnative {
//				continue
//			}
//			output = append(output, man)
//		}
//		return output, nil
//	}
//}
//
//var commentRegex = regexp.MustCompile("#.*")
//
//func IsEmptyManifest(manifest string) bool {
//	removeComments := commentRegex.ReplaceAllString(manifest, "")
//	removeNewlines := strings.Replace(removeComments, "\n", "", -1)
//	removeDashes := strings.Replace(removeNewlines, "---", "", -1)
//	return removeDashes == ""
//}
//
//func GetResources(manifest string) ([]ResourceType, error) {
//	var resources []ResourceType
//	for _, doc := range strings.Split(manifest, "---") {
//		if strings.TrimSpace(doc) == "" {
//			continue
//		}
//		var resource ResourceType
//		if err := yaml.Unmarshal([]byte(doc), &resource); err != nil {
//			return nil, errors.Wrapf(err, "parsing resource: %s", doc)
//		}
//		resources = append(resources, resource)
//	}
//	return resources, nil
//}
