package upgrade

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/go-utils/githubutils"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

var _ = Describe("upgrade utils unit tests", func() {

	Context("Should never fail if you have internet", func() {
		It("should error or have a nil lastminor", func() {
			lastminor, _, err := GetUpgradeVersions(context.Background(), "gloo")

			belief := err != nil || lastminor == nil
			Expect(belief).To(BeTrue())
		})
	})

	Context("knows how to handle certain github cases", func() {

		It("should return latest patch", func() {
			ctx := context.Background()
			client, _ := githubutils.GetClient(ctx)
			minor, err := getLatestReleasedPatchVersion(ctx, client, "gloo", 1, 8)
			Expect(err).NotTo(HaveOccurred())
			Expect(minor.String()).To(Equal("v1.8.37"))
		})
	})

})
