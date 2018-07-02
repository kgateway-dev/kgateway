package nats

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/helpers/local"
)

func TestNatsStreaming(t *testing.T) {
	RegisterFailHandler(Fail)
	log.DefaultOut = GinkgoWriter
	RunSpecs(t, "NatsStreaming Suite")
}

var (
	natsStreamingFactory *localhelpers.NatsStreamingFactory
	err                  error
)

var _ = BeforeSuite(func() {
	natsStreamingFactory, err = localhelpers.NewNatsStreamingFactory()
	helpers.Must(err)
})

var _ = AfterSuite(func() {
	natsStreamingFactory.Clean()
})

var (
	natsStreamingInstance *localhelpers.NatsStreamingInstance
)

var _ = BeforeEach(func() {
	natsStreamingInstance, err = natsStreamingFactory.NewNatsStreamingInstance()
	helpers.Must(err)
})

var _ = AfterEach(func() {
	natsStreamingInstance.Clean()
})
