package krtcollections

import (
	"github.com/solo-io/gloo/projects/gateway2/ir"
	"istio.io/istio/pkg/kube/krt"
)

// KRTExtensions allows appending to the core KRT collections used for XDS.
type KRTExtensions interface {
	Synced() krt.Syncer
	Endpoints() []krt.Collection[ir.EndpointsForUpstream]
	Upstreams() []krt.Collection[UpstreamWrapper]
}

var _ KRTExtensions = aggregate{}

// Aggregate will append the outputs of each extension
func Aggregate(
	extensions ...KRTExtensions,
) KRTExtensions {
	return aggregate(extensions)
}

type aggregate []KRTExtensions

func (a aggregate) Endpoints() (out []krt.Collection[ir.EndpointsForUpstream]) {
	for _, e := range a {
		out = append(out, e.Endpoints()...)
	}
	return out
}

func (a aggregate) Upstreams() (out []krt.Collection[UpstreamWrapper]) {
	for _, e := range a {
		out = append(out, e.Upstreams()...)
	}
	return out
}

func (a aggregate) HasSynced() bool {
	return a.Synced().HasSynced()
}

func (a aggregate) WaitUntilSynced(stop <-chan struct{}) bool {
	return a.Synced().WaitUntilSynced(stop)
}

func (a aggregate) Synced() krt.Syncer {
	syncers := FlattenedSyncers{}
	for _, c := range a.Endpoints() {
		syncers = append(syncers, c.Synced())
	}
	for _, c := range a.Upstreams() {
		syncers = append(syncers, c.Synced())
	}
	return syncers
}

var _ krt.Syncer = FlattenedSyncers{}

type FlattenedSyncers []krt.Syncer

func (f FlattenedSyncers) HasSynced() bool {
	for _, s := range f {
		if !s.HasSynced() {
			return false
		}
	}
	return true
}

func (f FlattenedSyncers) WaitUntilSynced(stop <-chan struct{}) bool {
	allSynced := true
	for _, k := range f {
		select {
		case <-stop:
			return false
		default:
			// don't return early if one is false
			allSynced = allSynced && k.WaitUntilSynced(stop)
		}
	}
	return allSynced
}
