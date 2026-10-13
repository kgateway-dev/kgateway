package krtcollections

import (
	"fmt"
)

// WatchSelector describes the label selector a Service, Secret, or ConfigMap watch was
// narrowed with, so that a lookup miss can name it. Excluded objects never reach the informer
// cache, so kgateway cannot tell an excluded object from a nonexistent one.
type WatchSelector struct {
	// Selector is the label selector; empty means the watch is unfiltered.
	Selector string
	// HelmValue and EnvVar name where the selector is configured.
	HelmValue string
	EnvVar    string
}

// ServiceWatchSelector, SecretWatchSelector, and ConfigMapWatchSelector describe the
// selectors set by Settings.ServiceLabelSelector, SecretLabelSelector, and
// ConfigMapLabelSelector.
func ServiceWatchSelector(selector string) WatchSelector {
	return WatchSelector{Selector: selector, HelmValue: "serviceLabelSelector", EnvVar: "KGW_SERVICE_LABEL_SELECTOR"}
}

func SecretWatchSelector(selector string) WatchSelector {
	return WatchSelector{Selector: selector, HelmValue: "secretLabelSelector", EnvVar: "KGW_SECRET_LABEL_SELECTOR"}
}

func ConfigMapWatchSelector(selector string) WatchSelector {
	return WatchSelector{Selector: selector, HelmValue: "configMapLabelSelector", EnvVar: "KGW_CONFIG_MAP_LABEL_SELECTOR"}
}

// notFound returns a NotFoundError for obj, wrapped in a SelectorNotFoundError when the watch
// is narrowed.
func (w WatchSelector) notFound(obj NotFoundError) error {
	if w.Selector == "" {
		return &obj
	}
	return &SelectorNotFoundError{NotFound: &obj, WatchSelector: w}
}

// SelectorNotFoundError is a NotFoundError for an object of a kind whose watch is narrowed by a
// label selector. The message names the selector and where it is configured.
type SelectorNotFoundError struct {
	NotFound *NotFoundError
	WatchSelector
}

func (e *SelectorNotFoundError) Error() string {
	kind := e.NotFound.NotFoundObj.Kind
	return fmt.Sprintf(
		"%s; kgateway only watches %ss matching label selector %q (Helm value %s, %s); verify the %s labels match the selector",
		e.NotFound, kind, e.Selector, e.HelmValue, e.EnvVar, kind,
	)
}

func (e *SelectorNotFoundError) Unwrap() error {
	return e.NotFound
}
