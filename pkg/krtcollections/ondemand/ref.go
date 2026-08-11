// Package ondemand provides a KRT collection that caches the full contents of
// only those cluster objects that kgateway configuration actually references.
//
// # Why
//
// kgateway needs the contents of a handful of Secrets and ConfigMaps: listener
// certificates, backend CA bundles, auth credentials and so on. Historically it
// watched every Secret and ConfigMap in the cluster with a typed informer, which
// keeps the full object -- including `data` -- in memory. In clusters with many
// large ConfigMaps that dominates the control plane's heap even though almost
// none of those objects are ever read.
//
// # How
//
// A Cache watches the resource cluster-wide with a *metadata* informer. A
// PartialObjectMetadata is a few hundred bytes regardless of how large the
// object's payload is, so the cluster-wide watch stays cheap. The metadata watch
// is only used to answer two questions: does this object exist, and has it
// changed (its resourceVersion moves on every write).
//
// Separately, the rest of kgateway publishes the set of objects its
// configuration references as a KRT collection of ResourceRef. For that set --
// and only that set -- the Cache issues a direct Get for the full object and
// publishes the result into a krt.StaticCollection that downstream collections
// consume exactly as they would an informer-backed one.
//
// The metadata watch is what makes this work without polling: it already tells
// us precisely when a referenced object changed, so a full re-Get is driven by
// the same event stream a typed informer would have used, minus the payload for
// the objects nobody asked for.
package ondemand

import (
	"fmt"
	"maps"
	"strings"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
)

// ResourceRef is a reference from some piece of kgateway configuration to a
// core object whose contents kgateway needs.
//
// A ref either names a single object (Name set) or selects objects by label
// (MatchLabels set). Exactly one of the two must be set.
//
// Refs are collected from every part of the system that reads object contents;
// an object that no ref points at is never fetched, and reading it will fail as
// if it did not exist. Adding a new place that reads a Secret or ConfigMap
// therefore also means contributing a ref for it -- see
// pluginsdk.Plugin.ContributesResourceRefs.
type ResourceRef struct {
	// Kind is the object kind, e.g. "Secret" or "ConfigMap". Core group only.
	Kind string

	// Namespace is the namespace of the referenced object. For a MatchLabels ref
	// an empty Namespace selects across all namespaces.
	Namespace string

	// Name is the name of a single referenced object. Mutually exclusive with
	// MatchLabels.
	Name string

	// MatchLabels selects referenced objects by label. Mutually exclusive with
	// Name.
	MatchLabels map[string]string

	// Source identifies the configuration object that produced this ref. It only
	// exists to keep ResourceName unique: two policies referencing the same
	// Secret must produce two distinct collection entries, otherwise KRT would
	// treat them as a conflicting duplicate key. The Cache unions refs by target,
	// so duplicates cost nothing beyond a map entry.
	Source string
}

// NewRef builds a ref to a single named object.
func NewRef(source, kind, namespace, name string) ResourceRef {
	return ResourceRef{Kind: kind, Namespace: namespace, Name: name, Source: source}
}

// NewSelectorRef builds a ref to every object matching matchLabels. An empty
// namespace selects across all namespaces.
func NewSelectorRef(source, kind, namespace string, matchLabels map[string]string) ResourceRef {
	return ResourceRef{Kind: kind, Namespace: namespace, MatchLabels: matchLabels, Source: source}
}

// ResourceName implements krt.ResourceNamer.
func (r ResourceRef) ResourceName() string {
	var b strings.Builder
	b.WriteString(r.Source)
	b.WriteString("~")
	b.WriteString(r.Kind)
	b.WriteString("/")
	b.WriteString(r.Namespace)
	b.WriteString("/")
	if r.Name != "" {
		b.WriteString(r.Name)
		return b.String()
	}
	// Selector refs have no name; key on the rendered selector so that a policy
	// with two different selectors yields two entries.
	b.WriteString("[")
	b.WriteString(labels.Set(r.MatchLabels).String())
	b.WriteString("]")
	return b.String()
}

// Equals implements the KRT equality contract. All fields are compared.
func (r ResourceRef) Equals(other ResourceRef) bool {
	return r.Kind == other.Kind &&
		r.Namespace == other.Namespace &&
		r.Name == other.Name &&
		r.Source == other.Source &&
		maps.Equal(r.MatchLabels, other.MatchLabels)
}

func (r ResourceRef) String() string {
	if r.Name != "" {
		return fmt.Sprintf("%s %s/%s (from %s)", r.Kind, r.Namespace, r.Name, r.Source)
	}
	ns := r.Namespace
	if ns == "" {
		ns = "*"
	}
	return fmt.Sprintf("%s %s/[%s] (from %s)", r.Kind, ns, labels.Set(r.MatchLabels).String(), r.Source)
}

// isSelector reports whether this ref selects by label rather than by name.
func (r ResourceRef) isSelector() bool {
	return r.Name == ""
}

// target returns the single object this ref names. Only valid when !isSelector.
func (r ResourceRef) target() types.NamespacedName {
	return types.NamespacedName{Namespace: r.Namespace, Name: r.Name}
}

// Dedupe removes refs that resolve to the same key.
//
// KRT requires the output of a single transformation to have unique keys, and
// duplicates are easy to produce honestly: two HTTPS listeners on one Gateway
// sharing a certificate, or two headers sourced from the same Secret. Every ref
// producer should pass its result through this.
func Dedupe(refs []ResourceRef) []ResourceRef {
	if len(refs) < 2 {
		return refs
	}
	seen := make(map[string]struct{}, len(refs))
	out := refs[:0]
	for _, r := range refs {
		key := r.ResourceName()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, r)
	}
	return out
}
