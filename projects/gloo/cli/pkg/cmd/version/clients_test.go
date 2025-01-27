package version

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/install/helm/gloo/generate"

	corev1 "k8s.io/api/core/v1"
)

var _ = Describe("container image parsing", func() {

	Context("validate parsing image", func() {
		testCases := []struct {
			name                    string
			container               corev1.Container
			expectedImageRegistry   string
			expectedImageRepository string
			expectedImageTag        string
			expectedImageDigest     string
		}{
			{
				name: "no_reg_with_latest_tag_found",
				container: corev1.Container{
					Image: "gloo",
				},
				expectedImageRegistry:   "",
				expectedImageRepository: "gloo",
				expectedImageTag:        "latest",
				expectedImageDigest:     "",
			},
			{
				name: "no_reg_with_valid_tag_found",
				container: corev1.Container{
					Image: "gloo:1.0.0",
				},
				expectedImageRegistry:   "",
				expectedImageRepository: "gloo",
				expectedImageTag:        "1.0.0",
				expectedImageDigest:     "",
			},
			{
				name: "no_reg_with_valid_digest_found",
				container: corev1.Container{
					Image: "gloo@sha256:eec124aab01d2cdc43f47b82012f5e15ecd5f61069786cecfe0f1c0267bb3c0d",
				},
				expectedImageRegistry:   "",
				expectedImageRepository: "gloo",
				expectedImageTag:        "latest",
				expectedImageDigest:     "eec124aab01d2cdc43f47b82012f5e15ecd5f61069786cecfe0f1c0267bb3c0d",
			},
			{
				name: "latest_image_tag_found",
				container: corev1.Container{
					Image: "quay.io/solo-io/gloo",
				},
				expectedImageRegistry:   "quay.io/solo-io",
				expectedImageRepository: "gloo",
				expectedImageTag:        "latest",
				expectedImageDigest:     "",
			},
			{
				name: "root_reg_with_valid_tag_found",
				container: corev1.Container{
					Image: "solo-io/gloo:1.0.0",
				},
				expectedImageRegistry:   "solo-io",
				expectedImageRepository: "gloo",
				expectedImageTag:        "1.0.0",
				expectedImageDigest:     "",
			},
			{
				name: "valid_image_tag_found",
				container: corev1.Container{
					Image: "quay.io/solo-io/gloo:1.0.0",
				},
				expectedImageRegistry:   "quay.io/solo-io",
				expectedImageRepository: "gloo",
				expectedImageTag:        "1.0.0",
				expectedImageDigest:     "",
			},
			{
				name: "latest_image_tag_with_valid_digest_found",
				container: corev1.Container{
					Image: "quay.io/solo-io/gloo@sha256:eec124aab01d2cdc43f47b82012f5e15ecd5f61069786cecfe0f1c0267bb3c0d",
				},
				expectedImageRegistry:   "quay.io/solo-io",
				expectedImageRepository: "gloo",
				expectedImageTag:        "latest",
				expectedImageDigest:     "eec124aab01d2cdc43f47b82012f5e15ecd5f61069786cecfe0f1c0267bb3c0d",
			},
			{
				name: "valid_image_tag_with_valid_digest_found",
				container: corev1.Container{
					Image: "quay.io/solo-io/gloo:1.0.0@sha256:eec124aab01d2cdc43f47b82012f5e15ecd5f61069786cecfe0f1c0267bb3c0d",
				},
				expectedImageRegistry:   "quay.io/solo-io",
				expectedImageRepository: "gloo",
				expectedImageTag:        "1.0.0",
				expectedImageDigest:     "eec124aab01d2cdc43f47b82012f5e15ecd5f61069786cecfe0f1c0267bb3c0d",
			},
			{
				name: "reg_port_numb_with_latest_tag_found",
				container: corev1.Container{
					Image: "solo-io:1010/gloo",
				},
				expectedImageRegistry:   "solo-io:1010",
				expectedImageRepository: "gloo",
				expectedImageTag:        "latest",
				expectedImageDigest:     "",
			},
			{
				name: "reg_port_numb_with_valid_tag_found",
				container: corev1.Container{
					Image: "solo-io:1010/gloo:1.0.0",
				},
				expectedImageRegistry:   "solo-io:1010",
				expectedImageRepository: "gloo",
				expectedImageTag:        "1.0.0",
				expectedImageDigest:     "",
			},
			{
				name: "reg_port_numb_with_latest_tag_valid_digest_found",
				container: corev1.Container{
					Image: "solo-io:1010/gloo@sha256:eec124aab01d2cdc43f47b82012f5e15ecd5f61069786cecfe0f1c0267bb3c0d",
				},
				expectedImageRegistry:   "solo-io:1010",
				expectedImageRepository: "gloo",
				expectedImageTag:        "latest",
				expectedImageDigest:     "eec124aab01d2cdc43f47b82012f5e15ecd5f61069786cecfe0f1c0267bb3c0d",
			},
			{
				name: "valid_non-semver_tag_found",
				container: corev1.Container{
					Image: "quay.io/solo-io/gloo:1.0.0-patch10",
				},
				expectedImageRegistry:   "quay.io/solo-io",
				expectedImageRepository: "gloo",
				expectedImageTag:        "1.0.0-patch10",
				expectedImageDigest:     "",
			},
		}

		for _, test := range testCases {
			test := test
			Context(test.name, func() {
				It("can parse container image string", func() {
					image := parseContainerString(test.container)
					expectedImage := &generate.Image{
						Tag:        &test.expectedImageTag,
						Digest:     &test.expectedImageDigest,
						Registry:   &test.expectedImageRegistry,
						Repository: &test.expectedImageRepository,
					}

					Expect(*image.Tag).To(Equal(*expectedImage.Tag))
					Expect(*image.Digest).To(Equal(*expectedImage.Digest))
					Expect(*image.Registry).To(Equal(*expectedImage.Registry))
					Expect(*image.Repository).To(Equal(*expectedImage.Repository))
				})
			})
		}
	})
})
