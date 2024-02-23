package xds_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
)

var _ = Describe("Cache", func() {

	It("SnapshotCacheKeys returns the keys formatted correctly", func() {
		owner1, owner2, namespace1, namespace2, name1, name2 := "owner1", "owner2", "namespace1", "namespace2", "name1", "name2"

		proxy1 := v1.NewProxy(namespace1, name1)
		proxy1.Metadata.Labels = map[string]string{
			utils.TranslatorKey: owner1,
		}

		proxy2 := v1.NewProxy(namespace2, name2)
		proxy2.Metadata.Labels = map[string]string{
			utils.TranslatorKey: owner2,
		}
		proxies := []*v1.Proxy{
			proxy1,
			proxy2,
		}
		expectedKeys := []string{fmt.Sprintf("%v~%v~%v", owner1, namespace1, name1), fmt.Sprintf("%v~%v~%v", owner2, namespace2, name2)}
		actualKeys := xds.SnapshotCacheKeys(proxies)
		Expect(actualKeys).To(BeEquivalentTo(expectedKeys))
	})
})
