package config_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	google_protobuf "github.com/gogo/protobuf/types"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	. "github.com/solo-io/gloo/pkg/config"
	"github.com/solo-io/gloo/pkg/storage"
	"github.com/solo-io/gloo/pkg/storage/file"
)

func makeTestUpstream(number int) *v1.Upstream {
	return &v1.Upstream{
		Name: fmt.Sprintf("test-us-%d", number),
	}
}

const OwnerTest = "test"

var _ = Describe("DesiredState", func() {
	var (
		syncer       *UpstreamSyncer
		glooClient   storage.Interface
		dir          string
		desiredState []*v1.Upstream
		desiredError error
	)
	BeforeEach(func() {
		var err error
		dir, err = ioutil.TempDir("", "DesiredStateTest")
		Expect(err).NotTo(HaveOccurred())
		glooClient, err = file.NewStorage(dir, time.Millisecond)
		Expect(err).NotTo(HaveOccurred())
		err = glooClient.V1().Register()
		Expect(err).NotTo(HaveOccurred())

		syncer = &UpstreamSyncer{
			GlooStorage:      glooClient,
			Owner:            OwnerTest,
			DesiredUpstreams: func() ([]*v1.Upstream, error) { return desiredState, desiredError },
		}
	})
	AfterEach(func() {
		if dir != "" {
			os.RemoveAll(dir)
		}
	})

	It("Should create upstreams when storage is empty", func() {
		origUs := makeTestUpstream(1)
		desiredState = append(desiredState, origUs)
		err := syncer.SyncDesiredState()
		Expect(err).NotTo(HaveOccurred())

		createdUpstreams, err := glooClient.V1().Upstreams().List()
		Expect(err).NotTo(HaveOccurred())
		Expect(createdUpstreams).To(HaveLen(1))

		us := createdUpstreams[0]
		Expect(us.Name).To(Equal(origUs.Name))
		Expect(us.Metadata.Annotations[OwnerAnnotationKey]).To(Equal(OwnerTest))

	})

	It("Should not create duplicate upstreams", func() {
		desiredState = append(desiredState, makeTestUpstream(1))
		err := syncer.SyncDesiredState()
		Expect(err).NotTo(HaveOccurred())
		err = syncer.SyncDesiredState()
		Expect(err).NotTo(HaveOccurred())

		createdUpstreams, err := glooClient.V1().Upstreams().List()
		Expect(err).NotTo(HaveOccurred())
		Expect(createdUpstreams).To(HaveLen(1))
	})

	It("Should remove stale upstreams", func() {
		desiredState = append(desiredState, makeTestUpstream(1))
		err := syncer.SyncDesiredState()
		Expect(err).NotTo(HaveOccurred())
		desiredState = nil
		err = syncer.SyncDesiredState()
		Expect(err).NotTo(HaveOccurred())

		createdUpstreams, err := glooClient.V1().Upstreams().List()
		Expect(err).NotTo(HaveOccurred())
		Expect(createdUpstreams).To(BeEmpty())
	})

	It("Should update same upstream on change", func() {
		origUs := makeTestUpstream(1)
		desiredState = append(desiredState, origUs)
		err := syncer.SyncDesiredState()
		Expect(err).NotTo(HaveOccurred())

		origUs.Spec = &google_protobuf.Struct{
			Fields: map[string]*google_protobuf.Value{
				"testvalue": &google_protobuf.Value{
					Kind: &google_protobuf.Value_StringValue{
						StringValue: "test",
					},
				},
			},
		}

		err = syncer.SyncDesiredState()
		Expect(err).NotTo(HaveOccurred())

		createdUpstreams, err := glooClient.V1().Upstreams().List()
		Expect(err).NotTo(HaveOccurred())
		Expect(createdUpstreams).To(HaveLen(1))

		us := createdUpstreams[0]
		Expect(us.Name).To(Equal(origUs.Name))
		Expect(us.Spec.Fields["testvalue"]).ToNot(BeNil())
	})

})
