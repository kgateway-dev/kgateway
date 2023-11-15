package testutils

import (
	"github.com/solo-io/gloo/v2/pkg/controller/scheme"
	"github.com/solo-io/gloo/v2/pkg/query"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func BuildGatewayQueries(
	objs []client.Object,
) query.GatewayQueries {
	builder := fake.NewClientBuilder().WithScheme(scheme.NewScheme())
	query.IterateIndices(func(o client.Object, f string, fun client.IndexerFunc) error {
		builder.WithIndex(o, f, fun)
		return nil
	})

	fakeClient := builder.WithObjects(objs...).Build()

	return query.NewData(fakeClient, scheme.NewScheme())

}
