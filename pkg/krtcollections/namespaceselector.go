package krtcollections

import (
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/labels"
)

func SameNamespace(ns string) func(kctx krt.HandlerContext, namespace string) bool {
	return func(kctx krt.HandlerContext, namespace string) bool {
		return ns == namespace
	}
}

func AllNamespace() func(kctx krt.HandlerContext, namespace string) bool {
	return func(kctx krt.HandlerContext, namespace string) bool {
		return true
	}
}

func NamespaceSelector(namespaces krt.Collection[NamespaceMetadata], sel labels.Selector) func(kctx krt.HandlerContext, namespace string) bool {
	return func(kctx krt.HandlerContext, namespace string) bool {
		ns := krt.FetchOne(kctx, namespaces, krt.FilterKey(namespace))
		return sel.Matches(labels.Set(ns.Labels))
	}
}

func NoNamespace() func(kctx krt.HandlerContext, namespace string) bool {
	return func(kctx krt.HandlerContext, namespace string) bool {
		return false
	}
}
