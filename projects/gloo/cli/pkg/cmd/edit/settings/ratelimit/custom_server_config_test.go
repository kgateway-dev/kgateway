package ratelimit_test

import (
	"io"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmdutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	ratelimitpb "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/plugins/ratelimit"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/utils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("CustomServerConfig", func() {
	var (
		settings       *gloov1.Settings
		settingsClient gloov1.SettingsClient
	)
	BeforeEach(func() {
		helpers.UseMemoryClients()
		// create a settings object
		settingsClient = helpers.MustSettingsClient()

		settings = &gloov1.Settings{
			Metadata: core.Metadata{
				Name:      "default",
				Namespace: "gloo-system",
			},
		}

		var err error
		settings, err = settingsClient.Write(settings, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())
	})

	Run := func(yaml string) error {

		cmdutils.EditFileForTest = func(prefix, suffix string, r io.Reader) ([]byte, string, error) {
			return []byte(yaml), "", nil
		}

		return testutils.Glooctl("edit settings --name default --namespace gloo-system ratelimit custom-server-config")
	}

	Validate := func(yaml string) *ratelimitpb.EnvoySettings_RateLimitCustomConfig {
		err := Run(yaml)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())

		settings, err = settingsClient.Read(settings.Metadata.Namespace, settings.Metadata.Name, clients.ReadOpts{})
		ExpectWithOffset(1, err).NotTo(HaveOccurred())

		var rlSettings ratelimitpb.EnvoySettings
		err = utils.UnmarshalExtension(settings, constants.EnvoyRateLimitExtensionName, &rlSettings)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())

		return rlSettings.CustomConfig
	}

	It("should parse example 1", func() {
		d := Validate(`
descriptors:
  - key: database
    value: users
    rate_limit:
      unit: second
      requests_per_unit: 500

  - key: database
    value: default
    rate_limit:
      unit: second
      requests_per_unit: 500`)

		expectDescriptor := []*ratelimitpb.Descriptor{
			{
				Key:   "database",
				Value: "users",
				RateLimit: &ratelimitpb.RateLimit{
					Unit:            ratelimitpb.RateLimit_SECOND,
					RequestsPerUnit: 500,
				},
			}, {
				Key:   "database",
				Value: "default",
				RateLimit: &ratelimitpb.RateLimit{
					Unit:            ratelimitpb.RateLimit_SECOND,
					RequestsPerUnit: 500,
				},
			},
		}

		Expect(d.Descriptors).To(BeEquivalentTo(expectDescriptor))
	})

	It("should parse example 2", func() {
		d := Validate(`
descriptors:
# Only allow 5 marketing messages a day
- key: message_type
  value: marketing
  descriptors:
    - key: to_number
      rate_limit:
        unit: day
        requests_per_unit: 5

# Only allow 100 messages a day to any unique phone number
- key: to_number
  rate_limit:
    unit: day
    requests_per_unit: 100`)

		expectDescriptor := []*ratelimitpb.Descriptor{
			{
				Key:   "message_type",
				Value: "marketing",
				Descriptors: []*ratelimitpb.Descriptor{{
					Key: "to_number",
					RateLimit: &ratelimitpb.RateLimit{
						Unit:            ratelimitpb.RateLimit_DAY,
						RequestsPerUnit: 5,
					},
				}},
			}, {
				Key: "to_number",
				RateLimit: &ratelimitpb.RateLimit{
					Unit:            ratelimitpb.RateLimit_DAY,
					RequestsPerUnit: 100,
				},
			},
		}
		Expect(d.Descriptors).To(BeEquivalentTo(expectDescriptor))
	})

	It("should parse example 3", func() {
		d := Validate(`descriptors:
- key: remote_address
  rate_limit:
    unit: second
    requests_per_unit: 10

# Black list IP
- key: remote_address
  value: 50.0.0.5
  rate_limit:
    unit: second
    requests_per_unit: 0`)

		expectDescriptor := []*ratelimitpb.Descriptor{
			{
				Key: "remote_address",
				RateLimit: &ratelimitpb.RateLimit{
					Unit:            ratelimitpb.RateLimit_SECOND,
					RequestsPerUnit: 10,
				},
			}, {
				Key:   "remote_address",
				Value: "50.0.0.5",
				RateLimit: &ratelimitpb.RateLimit{
					Unit:            ratelimitpb.RateLimit_SECOND,
					RequestsPerUnit: 0,
				},
			},
		}
		Expect(d.Descriptors).To(BeEquivalentTo(expectDescriptor))
	})

	It("should parse example 4", func() {
		d := Validate(`
descriptors:
  - key: key
    value: value
    descriptors:
      - key: subkey
        rate_limit:
             requests_per_unit: 300
             unit: second
`)

		expectDescriptor := []*ratelimitpb.Descriptor{
			{
				Key:   "key",
				Value: "value",
				Descriptors: []*ratelimitpb.Descriptor{{
					Key: "subkey",
					RateLimit: &ratelimitpb.RateLimit{
						Unit:            ratelimitpb.RateLimit_SECOND,
						RequestsPerUnit: 300,
					},
				}},
			},
		}
		Expect(d.Descriptors).To(BeEquivalentTo(expectDescriptor))
	})

	It("should not allow non existing fields", func() {
		d := `
domain: mongo_cps
descriptors:
  - key: database
    value: users
    rate_limit:
      unit: second
      requests_per_unit: 500
  - key: database
    value: default
    rate_limit:
      unit: second
      requests_per_unit: 500`
		err := Run(d)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unknown field \"domain\""))
	})
})
