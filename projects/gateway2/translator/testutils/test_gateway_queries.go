package testutils

import (
	"github.com/solo-io/gloo/projects/gateway2/controller"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func BuildGatewayQueries(
	objs []client.Object,
) controller.GatewayQueries {
	builder := fake.NewClientBuilder().WithScheme(controller.NewScheme())
	controller.IterateIndices(func(o client.Object, f string, fun client.IndexerFunc) error {
		builder.WithIndex(o, f, fun)
		return nil
	})

	fakeClient := builder.WithObjects(objs...).Build()

	return controller.NewData(fakeClient, controller.NewScheme())

}
