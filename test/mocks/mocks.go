//go:build ignore

package mocks

//go:generate go run github.com/golang/mock/mockgen -destination ./cache/corecache.go github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache KubeCoreCache
//go:generate go run github.com/golang/mock/mockgen -destination ./kubernetes/kubeinterface.go k8s.io/client-go/kubernetes Interface
//go:generate go run github.com/golang/mock/mockgen -destination ./gloo/validation_client.go github.com/kgateway-dev/kgateway/internal/gloo/pkg/api/grpc/validation GlooValidationServiceClient
