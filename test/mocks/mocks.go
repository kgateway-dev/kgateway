package mocks

//go:generate mockgen -destination ./cache/corecache.go github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache KubeCoreCache
//go:generate mockgen -destination ./kubernetes/kubeinterface.go k8s.io/client-go/kubernetes Interface
