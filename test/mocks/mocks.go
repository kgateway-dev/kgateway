package mocks

//go:generate mockgen -destination ./cache/corecache.go github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache KubeCoreCache
//go:generate mockgen -destination ./kubernetes/kubeinterface.go k8s.io/client-go/kubernetes Interface
//go:generate mockgen -destination ./gloo/validation_client.go github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation GlooValidationServiceClient
