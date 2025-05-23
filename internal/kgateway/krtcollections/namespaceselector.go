package krtcollections

import (
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/labels"
)

func SameNamespace(ns string) func(krt.HandlerContext, string) bool {
	return func(_ krt.HandlerContext, s string) bool {
		return ns == s
	}
}

func AllNamespace() func(krt.HandlerContext, string) bool {
	return func(krt.HandlerContext, string) bool {
		return true
	}
}

func NamespaceSelector(namespaces krt.Collection[NamespaceMetadata], sel labels.Selector) func(krt.HandlerContext, string) bool {
	return func(kctx krt.HandlerContext, s string) bool {
		ns := krt.FetchOne(kctx, namespaces, krt.FilterKey(s))
		return sel.Matches(labels.Set(ns.Labels))
	}
}

func NoNamespace() func(krt.HandlerContext, string) bool {
	return func(krt.HandlerContext, string) bool {
		return false
	}
}
