package krtutil

import "istio.io/istio/pkg/kube/krt"

type KrtOptions struct {
	Stop     <-chan struct{}
	Debugger *krt.DebugHandler
}

func NewKrtOptions(stop <-chan struct{}, debugger *krt.DebugHandler) KrtOptions {
	return KrtOptions{
		Stop:     stop,
		Debugger: debugger,
	}
}

func (k KrtOptions) ToOptions(name string) []krt.CollectionOption {
	return []krt.CollectionOption{
		krt.WithName(name),
		krt.WithDebugging(k.Debugger),
		krt.WithStop(k.Stop),
	}
}
