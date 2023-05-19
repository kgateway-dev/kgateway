package validation

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

const (
	errorMessage   = "There was an error with the resource"
	warningMessage = "There was a warning with the resource"
)

var _ = Describe("Extension Validator", func() {
	var (
		extensionValidator *validator
		settings           v1.Settings
		ctx                context.Context
		snapshot           gloosnapshot.ApiSnapshot
		resource           resources.InputResource
		extensions         []syncer.TranslatorSyncerExtension
	)

	BeforeEach(func() {
		settings = v1.Settings{}
		resource = &v1.Upstream{
			Metadata: &core.Metadata{
				Namespace: "test-namespace",
				Name:      "upstreamName",
			},
		}

		ctx = context.TODO()
		snapshot = gloosnapshot.ApiSnapshot{}
		err := snapshot.UpsertToResourceList(resource)
		Expect(err).ToNot(HaveOccurred())
	})

	JustBeforeEach(func() {
		extensionValidator = NewValidator(extensions, &settings)
	})

	Context("failing extension validating Extensions", func() {
		BeforeEach(func() {
			extensions = []syncer.TranslatorSyncerExtension{
				TestFailingExtension{resource: resource},
			}
		})
		It("should fail on a failing extension", func() {
			reports := extensionValidator.Validate(ctx, &snapshot)
			err := reports.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(errorMessage))
			Expect(len(reports)).To(Equal(1))
		})

	})
	Context("passing extension", func() {
		BeforeEach(func() {
			extensions = []syncer.TranslatorSyncerExtension{
				TestPassingExtension{resource: resource},
			}
		})
		It("should pass on a passing extension", func() {
			reports := extensionValidator.Validate(ctx, &snapshot)
			err := reports.Validate()
			Expect(err).ToNot(HaveOccurred())
			Expect(len(reports)).To(Equal(1))
		})
	})

	Context("failing and passing extension", func() {
		BeforeEach(func() {
			extensions = []syncer.TranslatorSyncerExtension{
				TestPassingExtension{resource: resource},
				TestFailingExtension{resource: resource},
			}
		})
		It("should fail when one failing extension", func() {
			reports := extensionValidator.Validate(ctx, &snapshot)
			err := reports.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(errorMessage))
			Expect(len(reports)).To(Equal(1))
		})
	})

	Context("two passing extension", func() {
		BeforeEach(func() {
			extensions = []syncer.TranslatorSyncerExtension{
				TestPassingExtension{resource: resource},
				TestPassingExtension{resource: resource},
			}
		})
		It("should fail when one failing extension", func() {
			reports := extensionValidator.Validate(ctx, &snapshot)
			err := reports.Validate()
			Expect(err).ToNot(HaveOccurred())
			Expect(len(reports)).To(Equal(1))
		})
	})

	Context("one passing and one warning extension", func() {
		BeforeEach(func() {
			extensions = []syncer.TranslatorSyncerExtension{
				TestPassingExtension{resource: resource},
				TestWarningExtension{resource: resource},
			}
		})
		It("should show warning when one warning extension", func() {
			reports := extensionValidator.Validate(ctx, &snapshot)
			err := reports.ValidateStrict()
			Expect(err).To(HaveOccurred())
			Expect(len(reports)).To(Equal(1))
		})
	})

})

type TestFailingExtension struct {
	resource resources.InputResource
}

func (t TestFailingExtension) ID() string {
	return "failing Test"
}

func (t TestFailingExtension) Sync(ctx context.Context, snap *gloosnapshot.ApiSnapshot, settings *v1.Settings, snapshotSetter syncer.SnapshotSetter, reports reporter.ResourceReports) {
	reports.Accept(t.resource)
	reports.AddError(t.resource, errors.New(errorMessage))
}

type TestWarningExtension struct {
	resource resources.InputResource
}

func (t TestWarningExtension) ID() string {
	return "warning Test"
}

func (t TestWarningExtension) Sync(ctx context.Context, snap *gloosnapshot.ApiSnapshot, settings *v1.Settings, snapshotSetter syncer.SnapshotSetter, reports reporter.ResourceReports) {
	reports.Accept(t.resource)
	reports.AddWarning(t.resource, warningMessage)
}

type TestPassingExtension struct {
	resource resources.InputResource
}

func (t TestPassingExtension) ID() string {
	return "passing Test"
}

func (t TestPassingExtension) Sync(ctx context.Context, snap *gloosnapshot.ApiSnapshot, settings *v1.Settings, snapshotSetter syncer.SnapshotSetter, reports reporter.ResourceReports) {
	reports.Accept(t.resource)
}
