package ratelimit_test

import (
	"context"
	"io"

	"github.com/solo-io/solo-kit/test/matchers"

	rltypes "github.com/solo-io/solo-apis/pkg/api/ratelimit.solo.io/v1alpha1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmdutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	ratelimitpb "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/ratelimit"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("CustomServerConfig", func() {
	var (
		settings       *gloov1.Settings
		settingsClient gloov1.SettingsClient
		ctx            context.Context
		cancel         context.CancelFunc
	)
	BeforeEach(func() {
		helpers.UseMemoryClients()
		ctx, cancel = context.WithCancel(context.Background())
		// create a settings object
		settingsClient = helpers.MustSettingsClient(ctx)

		settings = &gloov1.Settings{
			Metadata: &core.Metadata{
				Name:      "default",
				Namespace: "gloo-system",
			},
		}

		var err error
		settings, err = settingsClient.Write(settings, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() { cancel() })

	Run := func(yaml string) error {

		cmdutils.EditFileForTest = func(prefix, suffix string, r io.Reader) ([]byte, string, error) {
			return []byte(yaml), "", nil
		}

		return testutils.Glooctl("edit settings --name default --namespace gloo-system ratelimit server-config")
	}

	Validate := func(yaml string) *ratelimitpb.ServiceSettings {
		err := Run(yaml)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())

		settings, err = settingsClient.Read(settings.Metadata.Namespace, settings.Metadata.Name, clients.ReadOpts{})
		ExpectWithOffset(1, err).NotTo(HaveOccurred())

		return settings.Ratelimit
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
      requests_per_unit: 500
setDescriptors:
  - simple_descriptors:
    - key: foo
      value: bar
    rate_limit:
      unit: second
      requests_per_unit: 50`)

		expectDescriptor := []*rltypes.Descriptor{
			{
				Key:   "database",
				Value: "users",
				RateLimit: &rltypes.RateLimit{
					Unit:            rltypes.RateLimit_SECOND,
					RequestsPerUnit: 500,
				},
			}, {
				Key:   "database",
				Value: "default",
				RateLimit: &rltypes.RateLimit{
					Unit:            rltypes.RateLimit_SECOND,
					RequestsPerUnit: 500,
				},
			},
		}
		expectSetDescriptor := []*rltypes.SetDescriptor{
			{
				SimpleDescriptors: []*rltypes.SimpleDescriptor{{
					Key:   "foo",
					Value: "bar",
				}},
				RateLimit: &rltypes.RateLimit{
					Unit:            rltypes.RateLimit_SECOND,
					RequestsPerUnit: 50,
				},
			},
		}

		for index, descriptor := range d.Descriptors {
			Expect(descriptor).To(matchers.MatchProto(expectDescriptor[index]))
		}
		for index, setDescriptor := range d.SetDescriptors {
			Expect(setDescriptor).To(matchers.MatchProto(expectSetDescriptor[index]))
		}
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

		expectDescriptor := []*rltypes.Descriptor{
			{
				Key:   "message_type",
				Value: "marketing",
				Descriptors: []*rltypes.Descriptor{{
					Key: "to_number",
					RateLimit: &rltypes.RateLimit{
						Unit:            rltypes.RateLimit_DAY,
						RequestsPerUnit: 5,
					},
				}},
			}, {
				Key: "to_number",
				RateLimit: &rltypes.RateLimit{
					Unit:            rltypes.RateLimit_DAY,
					RequestsPerUnit: 100,
				},
			},
		}
		for index, descriptor := range d.Descriptors {
			Expect(descriptor).To(matchers.MatchProto(expectDescriptor[index]))
		}
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

		expectDescriptor := []*rltypes.Descriptor{
			{
				Key: "remote_address",
				RateLimit: &rltypes.RateLimit{
					Unit:            rltypes.RateLimit_SECOND,
					RequestsPerUnit: 10,
				},
			}, {
				Key:   "remote_address",
				Value: "50.0.0.5",
				RateLimit: &rltypes.RateLimit{
					Unit:            rltypes.RateLimit_SECOND,
					RequestsPerUnit: 0,
				},
			},
		}
		for index, descriptor := range d.Descriptors {
			Expect(descriptor).To(matchers.MatchProto(expectDescriptor[index]))
		}
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

		expectDescriptor := []*rltypes.Descriptor{
			{
				Key:   "key",
				Value: "value",
				Descriptors: []*rltypes.Descriptor{{
					Key: "subkey",
					RateLimit: &rltypes.RateLimit{
						Unit:            rltypes.RateLimit_SECOND,
						RequestsPerUnit: 300,
					},
				}},
			},
		}
		for index, descriptor := range d.Descriptors {
			Expect(descriptor).To(matchers.MatchProto(expectDescriptor[index]))
		}
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
