package benchmark_test

import (
	"encoding/json"
	"fmt"

	"github.com/mitchellh/hashstructure"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/go-utils/protoutils"
)

var _ = Describe("SnapshotBenchmark", func() {
	var allUpstreams v1.UpstreamList
	BeforeEach(func() {
		var upstreams []interface{}
		data := helpers.MustReadFile("upstream_list.json")
		err := json.Unmarshal(data, &upstreams)
		Expect(err).NotTo(HaveOccurred())
		for _, usInt := range upstreams {
			usMap := usInt.(map[string]interface{})
			var us v1.Upstream
			err = protoutils.UnmarshalMap(usMap, &us)
			Expect(err).NotTo(HaveOccurred())
			allUpstreams = append(allUpstreams, &us)
		}

		// Sort merged slice for consistent hashing
		allUpstreams.Sort()

	})
	Measure("it should do something hard efficiently", func(b Benchmarker) {
		const times = 1
		reflectionBased := b.Time(fmt.Sprintf("runtime of %d reflect-based hash calls", times), func() {
			for i := 0; i < times; i++ {
				for _, us := range allUpstreams {
					hashstructure.Hash(us, nil)
				}
			}
		})
		generated := b.Time(fmt.Sprintf("runtime of %d generated hash calls", times), func() {
			for i := 0; i < times; i++ {
				for _, us := range allUpstreams {
					us.Hash(nil)
				}
			}
		})
		// divide by 1e3 to get time in micro seconds instead of nano seconds
		b.RecordValue("Runtime per reflection call in µ seconds", float64(int64(reflectionBased)/times)/1e3)
		b.RecordValue("Runtime per generated call in µ seconds", float64(int64(generated)/times)/1e3)

	}, 10)

	Context("accuracy", func() {
		It("Exhaustive", func() {
			present := make(map[uint64]*v1.Upstream, len(allUpstreams))
			for _, v := range allUpstreams {
				hash, err := v.Hash(nil)
				Expect(err).NotTo(HaveOccurred())
				val, ok := present[hash]
				if ok {
					Expect(v.UpstreamType.Equal(val.UpstreamType))
				}
				present[hash] = v
			}
		})
	})
})
