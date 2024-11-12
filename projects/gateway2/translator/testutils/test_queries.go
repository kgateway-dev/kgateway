package testutils

import (
	"github.com/solo-io/gloo/pkg/schemes"
	"github.com/solo-io/gloo/projects/gateway2/query"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type IndexIteratorFunc = func(f func(client.Object, string, client.IndexerFunc) error) error

func BuildIndexedFakeClient(objs []client.Object, funcs ...IndexIteratorFunc) client.Client {
	builder := fake.NewClientBuilder().WithScheme(schemes.DefaultScheme())
	for _, f := range funcs {
		f(func(o client.Object, s string, ifunc client.IndexerFunc) error {
			builder.WithIndex(o, s, ifunc)
			return nil
		})
	}

	return builder.WithObjects(objs...).Build()
}

func BuildGatewayQueriesWithClient(fakeClient client.Client) query.GatewayQueries {
	return query.NewData(fakeClient, schemes.DefaultScheme())
}

func BuildGatewayQueries(
	objs []client.Object,
) query.GatewayQueries {
	builder := fake.NewClientBuilder().WithScheme(schemes.DefaultScheme())
	query.IterateIndices(func(o client.Object, f string, fun client.IndexerFunc) error {
		builder.WithIndex(o, f, fun)
		return nil
	})

	fakeClient := builder.WithObjects(objs...).Build()

	return query.NewData(fakeClient, schemes.DefaultScheme())
}
