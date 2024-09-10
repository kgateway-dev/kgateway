package validation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/hashicorp/go-multierror"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/statsutils"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/translator"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	gloov1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer"
	syncerValidation "github.com/solo-io/gloo/projects/gloo/pkg/syncer/validation"
	validationutils "github.com/solo-io/gloo/projects/gloo/pkg/utils/validation"
	gloovalidation "github.com/solo-io/gloo/projects/gloo/pkg/validation"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/solo-io/gloo/test/samples"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/testutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
	skmocks "github.com/solo-io/solo-kit/test/mocks/v1"
	"go.opencensus.io/stats/view"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8syamlutil "sigs.k8s.io/yaml"
)

type validationFunc func(ctx context.Context, proxy *gloov1.Proxy, resource resources.Resource, shouldDelete bool) ([]*gloovalidation.GlooValidationReport, error)

var _ = Describe("Validator", func() {
	var (
		t  translator.Translator
		gv GlooValidatorFunc
		ns string
		v  *validator
	)

	const glooValidationError = "with gloo validation"

	BeforeEach(func() {
		ns = "gloo-system"
		t = translator.NewDefaultTranslator(translator.Opts{
			WriteNamespace: ns,
		})

		extensionValidator := syncerValidation.NewValidator([]syncer.TranslatorSyncerExtension{}, nil)
		v = NewValidator(ValidatorConfig{
			Translator:         t,
			GlooValidator:      gv,
			ExtensionValidator: extensionValidator,
			AllowWarnings:      false,
		})
		mValidConfig = statsutils.MakeGauge("validation.gateway.solo.io/valid_config", "A boolean indicating whether gloo config is valid")
	})

	It("returns error before sync called", func() {
		_, err := v.ValidateModifiedGvk(context.TODO(), v1.GatewayGVK, nil, false)
		Expect(err).To(testutils.HaveInErrorChain(NotReadyErr))
		err = v.Sync(context.Background(), &gloov1snap.ApiSnapshot{})
		Expect(err).NotTo(HaveOccurred())
	})

	It("has mValidConfig=1 after Sync is called with valid snapshot", func() {
		err := v.Sync(context.TODO(), &gloov1snap.ApiSnapshot{})
		Expect(err).NotTo(HaveOccurred())

		rows, err := view.RetrieveData("validation.gateway.solo.io/valid_config")
		Expect(err).NotTo(HaveOccurred())
		Expect(rows).NotTo(BeEmpty())
		Expect(rows[0].Data.(*view.LastValueData).Value).To(BeEquivalentTo(1))
	})

	It("has mValidConfig=0 after Sync is called with invalid snapshot", func() {
		snap := samples.SimpleGlooSnapshot(ns)
		snap.Gateways.Each(func(element *v1.Gateway) {
			http, ok := element.GatewayType.(*v1.Gateway_HttpGateway)
			if !ok {
				return
			}
			http.HttpGateway.VirtualServiceExpressions = &v1.VirtualServiceSelectorExpressions{
				Expressions: []*v1.VirtualServiceSelectorExpressions_Expression{
					{
						Key:      "a",
						Operator: v1.VirtualServiceSelectorExpressions_Expression_Equals,
						Values:   []string{"b", "c"},
					},
				},
			}
		})
		err := v.Sync(context.TODO(), snap)
		Expect(err).To(HaveOccurred())

		rows, err := view.RetrieveData("validation.gateway.solo.io/valid_config")
		Expect(err).NotTo(HaveOccurred())
		Expect(rows).NotTo(BeEmpty())
		Expect(rows[0].Data.(*view.LastValueData).Value).To(BeEquivalentTo(0))
	})

	Context("validating gloo resources", func() {
		Context("upstreams", func() {

			It("accepts an upstream when validation succeeds", func() {
				v.glooValidator = ValidateAccept
				us := samples.SimpleUpstream()
				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				reports, err := v.ValidateModifiedGvk(context.TODO(), gloov1.UpstreamGVK, us, false)
				Expect(err).NotTo(HaveOccurred())
				Expect(*(reports.ProxyReports)).To(HaveLen(1))
				proxyReport := (*reports.ProxyReports)[0]
				warnings := validationutils.GetProxyWarning(proxyReport)
				errors := validationutils.GetProxyError(proxyReport)
				Expect(warnings).To(BeEmpty())
				Expect(errors).NotTo(HaveOccurred())
			})
			It("rejects an upstream when validation fails", func() {
				v.glooValidator = ValidateFail
				us := samples.SimpleUpstream()
				us.Metadata.Namespace = "my-namespace"
				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				reports, err := v.ValidateModifiedGvk(context.TODO(), gloov1.UpstreamGVK, us, false)
				Expect(err).To(HaveOccurred())
				Expect(*(reports.ProxyReports)).To(HaveLen(1))
				proxyReport := (*reports.ProxyReports)[0]
				errors := validationutils.GetProxyError(proxyReport)
				Expect(errors).To(HaveOccurred())
			})
			It("accepts an upstream when there is a validation warning and allowWarnings is true", func() {
				v.glooValidator = ValidateWarn
				v.allowWarnings = true

				us := samples.SimpleUpstream()
				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				reports, err := v.ValidateModifiedGvk(context.TODO(), gloov1.UpstreamGVK, us, false)
				Expect(err).NotTo(HaveOccurred())
				Expect(*(reports.ProxyReports)).To(HaveLen(1))
				proxyReport := (*reports.ProxyReports)[0]
				warnings := validationutils.GetProxyWarning(proxyReport)
				Expect(warnings).NotTo(BeEmpty())
			})
			It("rejects an upstream when there is a validation warning and allowWarnings is false", func() {
				v.glooValidator = ValidateWarn
				v.allowWarnings = false

				us := samples.SimpleUpstream()
				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				reports, err := v.ValidateModifiedGvk(context.TODO(), gloov1.UpstreamGVK, us, false)
				Expect(err).To(HaveOccurred())
				Expect(*(reports.ProxyReports)).To(HaveLen(1))
				proxyReport := (*reports.ProxyReports)[0]
				warnings := validationutils.GetProxyWarning(proxyReport)
				Expect(warnings).NotTo(BeEmpty())
			})

			It("accepts an upstream deletion when validation succeeds", func() {
				v.glooValidator = ValidateAccept

				us := samples.SimpleUpstream()
				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				err = v.ValidateDeletedGvk(context.TODO(), gloov1.UpstreamGVK, us, false)
				Expect(err).NotTo(HaveOccurred())
			})
			It("rejects an upstream deletion when validation fails", func() {
				v.glooValidator = ValidateFail

				us := samples.SimpleUpstream()
				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				err = v.ValidateDeletedGvk(context.TODO(), gloov1.UpstreamGVK, us, false)
				Expect(err).To(HaveOccurred())
			})
			It("accepts an upstream deletion when there is a validation warning and allowWarnings is true", func() {
				v.glooValidator = ValidateWarn
				v.allowWarnings = true

				us := samples.SimpleUpstream()
				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				err = v.ValidateDeletedGvk(context.TODO(), gloov1.UpstreamGVK, us, false)
				Expect(err).NotTo(HaveOccurred())
			})
			It("rejects an upstream deletion when there is a validation warning and allowWarnings is false", func() {
				v.glooValidator = ValidateWarn
				v.allowWarnings = false

				us := samples.SimpleUpstream()
				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				err = v.ValidateDeletedGvk(context.TODO(), gloov1.UpstreamGVK, us, false)
				Expect(err).To(HaveOccurred())
			})
		})

		Context("secret deletion", func() {
			// Inputs:
			// - disableValidationAgainstPreviousState bool - are we enabling the validation against the previous state or just checking errors/warnings as usual
			// - allowWarnings bool - are warnings allowed
			// - validator - Errors/Warnings/Success - what is returned from validation.
			// - errExpected bool - is an error expected
			// the glooValidator returns two types of reports: ProxyReports and ResourceReports.
			// Test that both are handled correctly. This test is focuses on the ProxyReports.
			DescribeTable("handles secret validation scenarios for glooValidation output", func(disableValidationAgainstPreviousState bool, allowWarnings bool, validator validationFunc, expectSuccess bool) {
				v.glooValidator = validator
				v.allowWarnings = allowWarnings
				v.disableValidationAgainstPreviousState = disableValidationAgainstPreviousState
				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				secret := &gloov1.Secret{
					Metadata: &core.Metadata{
						Name:      "secret",
						Namespace: "namespace",
					},
				}
				err = v.ValidateDeletedGvk(context.TODO(), gloov1.SecretGVK, secret, false)
				if expectSuccess {
					Expect(err).NotTo(HaveOccurred())
				} else {
					Expect(err).To(HaveOccurred())
				}
			},
				// Regular validation
				Entry("No snapshot comparison, allowWarnings=true, no errors or warnings, should succeed", true, true, ValidateAccept, true),
				Entry("No snapshot comparison, allowWarnings=true, errors w/ no warnings, should fail", true, true, ValidateFail, false),
				Entry("No snapshot comparison, allowWarnings=true, no errors w/ warnings, should succeed", true, true, ValidateWarn, true),
				Entry("No snapshot comparison, allowWarnings=true, errors and warnings, should fail", true, true, ValidationErrorAndWarn, false),
				Entry("No snapshot comparison, allowWarnings=false, no errors or warnings, should succeed", true, false, ValidateAccept, true),
				Entry("No snapshot comparison, allowWarnings=false, errors w/ no warnings, should fail", true, false, ValidateFail, false),
				Entry("No snapshot comparison, allowWarnings=false, no errors w/ warnings, should fail", true, false, ValidateWarn, false),
				Entry("No snapshot comparison, allowWarnings=false, errors and warnings, should fail", true, false, ValidationErrorAndWarn, false),
				// Snapshot comparison validation - cases where output is the same. These should all succeed
				Entry("Snapshot comparison, allowWarnings=true, no errors or warnings, consistent, should succeed", false, true, ValidateAccept, true),
				Entry("Snapshot comparison, allowWarnings=true, errors w/ no warnings, consistent, should succeed", false, true, ValidateFail, true),
				Entry("Snapshot comparison, allowWarnings=true, no errors w/ warnings, consistent, should succeed", false, true, ValidateWarn, true),
				Entry("Snapshot comparison, allowWarnings=true, errors and warnings, consistent, should succeed", false, true, ValidationErrorAndWarn, true),
				Entry("Snapshot comparison, allowWarnings=false, no errors or warnings, consistent, should succeed", false, false, ValidateAccept, true),
				Entry("Snapshot comparison, allowWarnings=false, errors w/ no warnings, consistent, should fail", false, false, ValidateFail, true),
				Entry("Snapshot comparison, allowWarnings=false, no errors w/ warnings, consistent, should fail", false, false, ValidateWarn, true),
				Entry("Snapshot comparison, allowWarnings=false, errors and warnings, consistent, should succeed", false, true, ValidationErrorAndWarn, true),
				// Snapshot comparison validation - cases where output changes - warnings are allowed
				Entry("Snapshot comparison, allowWarnings=true, errors w/ no warnings, errors changed, should fail", false, true, ValidateFailChangeError, false),
				Entry("Snapshot comparison, allowWarnings=true, no errors w/ warnings, warnings changed, should succeed", false, true, ValidateChangeWarning, true),
				Entry("Snapshot comparison, allowWarnings=true, no errors w/ warnings, warnings and errors changed, should fail", false, true, ValidateChangeWarningAndChangeError, false),
				Entry("Snapshot comparison, allowWarnings=true, errors and warnings, only errors changed, should fail", false, true, ValidateSameWarningAndChangeError, false),
				Entry("Snapshot comparison, allowWarnings=true, errors and warnings, only warnings changed, should pass", false, true, ValidateChangeWarningAndSameError, true),
				Entry("Snapshot comparison, allowWarnings=false, errors w/ no warnings, errors changed, should fail", false, false, ValidateFailChangeError, false),
				Entry("Snapshot comparison, allowWarnings=false, no errors w/ warnings, warnings changed, should fail", false, false, ValidateChangeWarning, false),
				Entry("Snapshot comparison, allowWarnings=false, no errors w/ warnings, warnings and errors changed, should fail", false, true, ValidateChangeWarningAndChangeError, false),
				Entry("Snapshot comparison, allowWarnings=false, errors and warnings, only errors changed, should fail", false, false, ValidateSameWarningAndChangeError, false),
				Entry("Snapshot comparison, allowWarnings=true, errors and warnings, only warnings changed, should fail", false, false, ValidateChangeWarningAndSameError, false),
			)

			// reportValidationEntries is a table of inputs for the validation tests, that can be resued to test handling of the gloo Translator,
			// the extension validator, and the glooValidator resourceReports,  as they all rely on resourceReports that can be generated the same way
			// and used appropriately in the separate tests.
			reportValidationEntries := []TableEntry{
				// Regular validation
				Entry("No snapshot comparison, allowWarnings=true, no errors or warnings, should succeed", true, true, generateNone, true),
				Entry("No snapshot comparison, allowWarnings=true, errors w/ no warnings, should fail", true, true, generateError, false),
				Entry("No snapshot comparison, allowWarnings=true, no errors w/ warnings, should succeed", true, true, generateWarn, true),
				Entry("No snapshot comparison, allowWarnings=true, errors and warnings, should fail", true, true, generateErrorAndWarn, false),
				Entry("No snapshot comparison, allowWarnings=false, no errors or warnings, should succeed", true, false, generateNone, true),
				Entry("No snapshot comparison, allowWarnings=false, errors w/ no warnings, should fail", true, false, generateError, false),
				Entry("No snapshot comparison, allowWarnings=false, no errors w/ warnings, should fail", true, false, generateWarn, false),
				Entry("No snapshot comparison, allowWarnings=false, errors and warnings, should fail", true, false, generateErrorAndWarn, false),
				// Snapshot comparison validation - cases where output is the same. These should all succeed
				Entry("Snapshot comparison, allowWarnings=true, no errors or warnings, consistent, should succeed", false, true, generateNone, true),
				Entry("Snapshot comparison, allowWarnings=true, errors w/ no warnings, consistent, should succeed", false, true, generateError, true),
				Entry("Snapshot comparison, allowWarnings=true, no errors w/ warnings, consistent, should succeed", false, true, generateWarn, true),
				Entry("Snapshot comparison, allowWarnings=true, errors and warnings, consistent, should succeed", false, true, generateErrorAndWarn, true),
				Entry("Snapshot comparison, allowWarnings=false, no errors or warnings, consistent, should succeed", false, false, generateNone, true),
				Entry("Snapshot comparison, allowWarnings=false, errors w/ no warnings, consistent, should fail", false, false, generateError, true),
				Entry("Snapshot comparison, allowWarnings=false, no errors w/ warnings, consistent, should fail", false, false, generateWarn, true),
				Entry("Snapshot comparison, allowWarnings=false, errors and warnings, consistent, should succeed", false, true, generateErrorAndWarn, true),
				// Snapshot comparison validation - cases where output changes - warnings are allowed
				Entry("Snapshot comparison, allowWarnings=true, errors w/ no warnings, errors changed, should fail", false, true, generateChangeError, false),
				Entry("Snapshot comparison, allowWarnings=true, no errors w/ warnings, warnings changed, should succeed", false, true, generateChangeWarn, true),
				Entry("Snapshot comparison, allowWarnings=true, no errors w/ warnings, warnings and errors changed, should fail", false, true, generateChangeWarnChangeError, false),
				Entry("Snapshot comparison, allowWarnings=true, errors and warnings, only errors changed, should fail", false, true, generateSameWarnChangeError, false),
				Entry("Snapshot comparison, allowWarnings=true, errors and warnings, only warnings changed, should pass", false, true, generateChangeWarnSameError, true),
				Entry("Snapshot comparison, allowWarnings=false, errors w/ no warnings, errors changed, should fail", false, false, generateChangeError, false),
				Entry("Snapshot comparison, allowWarnings=false, no errors w/ warnings, warnings changed, should fail", false, false, generateChangeWarn, false),
				Entry("Snapshot comparison, allowWarnings=false, no errors w/ warnings, warnings and errors changed, should fail", false, true, generateChangeWarnChangeError, false),
				Entry("Snapshot comparison, allowWarnings=false, errors and warnings, only errors changed, should fail", false, false, generateSameWarnChangeError, false),
				Entry("Snapshot comparison, allowWarnings=true, errors and warnings, only warnings changed, should fail", false, false, generateChangeWarnSameError, false),
			}

			DescribeTable("handles secret deletion for gloo translation scenarios with reports", func(disableValidationAgainstPreviousState bool, allowWarnings bool, reportGenerator func() reporter.ResourceReports, expectSuccess bool) {
				v.glooValidator = ValidateAccept
				v.allowWarnings = allowWarnings
				v.disableValidationAgainstPreviousState = disableValidationAgainstPreviousState

				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				// Update the translator after sync:
				v.translator = &MockTranslator{
					Translator:      t,
					ReportGenerator: reportGenerator,
				}

				secret := &gloov1.Secret{
					Metadata: &core.Metadata{
						Name:      "secret",
						Namespace: "namespace",
					},
				}
				err = v.ValidateDeletedGvk(context.TODO(), gloov1.SecretGVK, secret, false)
				if expectSuccess {
					Expect(err).NotTo(HaveOccurred())
				} else {
					Expect(err).To(HaveOccurred())
				}
			},
				reportValidationEntries,
			)

			DescribeTable("handles secret deletion for gloovalidation reports", func(disableValidationAgainstPreviousState bool, allowWarnings bool, reportGenerator func() reporter.ResourceReports, expectSuccess bool) {
				v.glooValidator = ValidationWithResourceReports(reportGenerator)
				v.allowWarnings = allowWarnings
				v.disableValidationAgainstPreviousState = disableValidationAgainstPreviousState

				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				secret := &gloov1.Secret{
					Metadata: &core.Metadata{
						Name:      "secret",
						Namespace: "namespace",
					},
				}
				err = v.ValidateDeletedGvk(context.TODO(), gloov1.SecretGVK, secret, false)
				if expectSuccess {
					Expect(err).NotTo(HaveOccurred())
				} else {
					Expect(err).To(HaveOccurred())
				}
			},
				reportValidationEntries,
			)

			DescribeTable("handles secret deletion for extension validation scenarios", func(disableValidationAgainstPreviousState bool, allowWarnings bool, reportGenerator func() reporter.ResourceReports, expectSuccess bool) {
				v.glooValidator = ValidateAccept
				v.allowWarnings = allowWarnings
				v.disableValidationAgainstPreviousState = disableValidationAgainstPreviousState

				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				// Update the validator after sync
				v.extensionValidator = &MockExtensionValidator{
					ReportGenerator: reportGenerator,
				}

				secret := &gloov1.Secret{
					Metadata: &core.Metadata{
						Name:      "secret",
						Namespace: "namespace",
					},
				}
				err = v.ValidateDeletedGvk(context.TODO(), gloov1.SecretGVK, secret, false)
				if expectSuccess {
					Expect(err).NotTo(HaveOccurred())
				} else {
					Expect(err).To(HaveOccurred())
				}
			},
				// Regular validation
				reportValidationEntries,
			)

			DescribeTable("Breaking errors don't trigger revalidation", func(validator validationFunc, expectedErrString string) {
				var (
					buf    *bytes.Buffer
					bws    *zapcore.BufferedWriteSyncer
					logger *zap.SugaredLogger
				)

				v.glooValidator = ValidationWithResourceReports(generateError)

				// The error we are interested in is not returned, but there is a relevant log message,
				// so capture/check that after the validation is run
				buf = &bytes.Buffer{}
				bws = &zapcore.BufferedWriteSyncer{WS: zapcore.AddSync(buf)}
				encoderCfg := zapcore.EncoderConfig{
					MessageKey:     "msg",
					LevelKey:       "level",
					NameKey:        "logger",
					EncodeLevel:    zapcore.LowercaseLevelEncoder,
					EncodeTime:     zapcore.ISO8601TimeEncoder,
					EncodeDuration: zapcore.StringDurationEncoder,
				}
				zapCore := zapcore.NewCore(zapcore.NewJSONEncoder(encoderCfg), bws, zap.DebugLevel)
				logger = zap.New(zapCore).Sugar()
				contextutils.SetFallbackLogger(logger)

				v.glooValidator = validator
				v.allowWarnings = true
				v.disableValidationAgainstPreviousState = false

				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				secret := &gloov1.Secret{
					Metadata: &core.Metadata{
						Name:      "secret",
						Namespace: "namespace",
					},
				}
				err = v.ValidateDeletedGvk(context.TODO(), gloov1.SecretGVK, secret, false)
				Expect(err).To(HaveOccurred())
				// Check the expected error was returned, but that is not enough to ensure the validation was not re-run
				Expect(err.Error()).To(ContainSubstring(expectedErrString))

				// Check the logs that the breaking error was recognized
				logerr := logger.Sync()
				Expect(logerr).NotTo(HaveOccurred())

				Expect(buf.String()).To(ContainSubstring(BreakingErrorLogMsg))
			},
				Entry("Sync not run error", ValidateNoGlooSync, gloovalidation.SyncNotCalledError.Error()),
				Entry("Validation Length Response Error", ValidateResponseLengthError, GlooValidationResponseLengthError{}.Error()),
			)
		})

	})

	Context("validating a route table", func() {
		Context("proxy validation accepted", func() {
			It("accepts the rt", func() {
				v.glooValidator = ValidateAccept
				snap := samples.GlooSnapshotWithDelegates(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				reports, err := v.ValidateModifiedGvk(context.TODO(), v1.RouteTableGVK, snap.RouteTables[0], false)
				Expect(err).NotTo(HaveOccurred())
				Expect(*(reports.ProxyReports)).To(HaveLen(1))
			})

			It("accepts the rt and returns proxies each time", func() {
				v.glooValidator = ValidateAccept
				snap := samples.GlooSnapshotWithDelegates(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				reports, err := v.ValidateModifiedGvk(context.TODO(), v1.RouteTableGVK, snap.RouteTables[0], false)
				Expect(err).NotTo(HaveOccurred())
				proxyReports := *(reports.ProxyReports)
				Expect(proxyReports).To(HaveLen(1))
				Expect(reports.GetProxies()).To(HaveLen(1))
				Expect(reports.GetProxies()[0]).To(ContainSubstring("listener-::-8080"))

				// repeat to ensure any hashing doesn't short circuit returning the proxies
				reports, err = v.ValidateModifiedGvk(context.TODO(), v1.RouteTableGVK, snap.RouteTables[0], false)
				Expect(err).NotTo(HaveOccurred())

				proxyReports = *(reports.ProxyReports)
				Expect(proxyReports).To(HaveLen(1))
				Expect(reports.GetProxies()).To(HaveLen(1))
				Expect(reports.GetProxies()[0]).To(ContainSubstring("listener-::-8080"))
			})
		})

		Context("proxy validation returns error", func() {
			It("rejects the rt", func() {
				v.glooValidator = ValidateFail
				snap := samples.GlooSnapshotWithDelegates(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				// change something to change the hash
				snap.RouteTables[0].Metadata.Labels = map[string]string{"change": "my mind"}

				reports, err := v.ValidateModifiedGvk(context.TODO(), v1.RouteTableGVK, snap.RouteTables[0], false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(glooValidationError))
				Expect(*(reports.ProxyReports)).To(HaveLen(1))
			})

			Context("allowWarnings=false", func() {
				It("rejects a vs with missing route table ref", func() {
					v.glooValidator = ValidateWarn
					snap := samples.GlooSnapshotWithDelegates(ns)
					err := v.Sync(context.TODO(), snap)
					Expect(err).NotTo(HaveOccurred())

					// change something to change the hash
					snap.RouteTables[0].Metadata.Labels = map[string]string{"change": "my mind"}

					rt := snap.RouteTables[0]
					reports, err := v.ValidateModifiedGvk(context.TODO(), rt.GroupVersionKind(), rt, false)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("Route Warning: InvalidDestinationWarning. Reason: " + warnString))
					Expect(*(reports.ProxyReports)).To(HaveLen(1))
				})
			})
		})

		Context("route table rejected", func() {
			It("rejects the rt", func() {
				badRoute := &v1.Route{
					Action: &v1.Route_DelegateAction{
						DelegateAction: &v1.DelegateAction{
							DelegationType: &v1.DelegateAction_Ref{
								Ref: &core.ResourceRef{
									Name:      "invalid",
									Namespace: "name",
								},
							},
						},
					},
				}

				// validate proxy should never be called
				v.glooValidator = nil
				snap := samples.GlooSnapshotWithDelegates(ns)
				rt := snap.RouteTables[0].DeepCopyObject().(*v1.RouteTable)
				rt.Routes = append(rt.Routes, badRoute)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				reports, err := v.ValidateModifiedGvk(context.TODO(), v1.RouteTableGVK, rt, false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("could not render proxy"))
				Expect(*(reports.ProxyReports)).To(BeEmpty())
			})
		})

		Context("route table delegation with selectors", func() {
			It("accepts route table with valid prefix", func() {
				v.glooValidator = ValidateAccept
				snap := samples.GatewaySnapshotWithDelegateSelector(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				rt := samples.RouteTableWithLabelsAndPrefix("route2", ns, "/foo/2", map[string]string{"pick": "me"})
				_, err = v.ValidateModifiedGvk(context.TODO(), v1.RouteTableGVK, rt, false)
				Expect(err).NotTo(HaveOccurred())
			})

			It("rejects route table with invalid prefix", func() {
				v.glooValidator = ValidateAccept
				snap := samples.GatewaySnapshotWithDelegateSelector(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				// the prefix doesn't start with the parent's prefix so validation will fail
				rt := samples.RouteTableWithLabelsAndPrefix("route2", ns, "/not", map[string]string{"pick": "me"})
				_, err = v.ValidateModifiedGvk(context.TODO(), v1.RouteTableGVK, rt, false)
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Context("delete a route table", func() {
		Context("has parents", func() {
			It("rejects deletion", func() {
				v.glooValidator = ValidateAccept
				snap := samples.GatewaySnapshotWithDelegateChain(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				err = v.ValidateDeletedGvk(context.TODO(), v1.RouteTableGVK, snap.RouteTables[1], false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("missing route table"))
			})
		})
		Context("has no parents", func() {
			It("deletes safely", func() {
				v.glooValidator = ValidateAccept
				snap := samples.GatewaySnapshotWithDelegateChain(ns)
				// break the parent chain
				snap.RouteTables[1].Routes = nil
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				err = v.ValidateDeletedGvk(context.TODO(), v1.RouteTableGVK, snap.RouteTables[2], false)
				Expect(err).NotTo(HaveOccurred())

				// ensure route table was removed from validator internal snapshot
				_, err = v.latestSnapshot.RouteTables.Find(snap.RouteTables[2].Metadata.Ref().Strings())
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Context("validating a virtual service", func() {

		Context("proxy validation returns error", func() {
			It("rejects the vs", func() {
				v.glooValidator = ValidateFail
				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				// change something to change the hash
				snap.VirtualServices[0].Metadata.Labels = map[string]string{"change": "my mind"}

				reports, err := v.ValidateModifiedGvk(context.TODO(), v1.VirtualServiceGVK, snap.VirtualServices[0], false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(glooValidationError))
				Expect(*(reports.ProxyReports)).To(HaveLen(1))

			})
		})
		Context("proxy validation accepted", func() {
			It("accepts the vs", func() {
				v.glooValidator = ValidateAccept
				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				reports, err := v.ValidateModifiedGvk(context.TODO(), v1.VirtualServiceGVK, snap.VirtualServices[0], false)
				Expect(err).NotTo(HaveOccurred())
				Expect(*(reports.ProxyReports)).To(HaveLen(1))
			})

			It("accepts the vs and returns proxies each time", func() {
				v.glooValidator = ValidateAccept
				snap := samples.GlooSnapshotWithDelegates(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				reports, err := v.ValidateModifiedGvk(context.TODO(), v1.VirtualServiceGVK, snap.VirtualServices[0], false)
				Expect(err).NotTo(HaveOccurred())
				proxyReports := *(reports.ProxyReports)
				Expect(proxyReports).To(HaveLen(1))
				Expect(reports.GetProxies()).To(HaveLen(1))
				Expect(reports.GetProxies()[0]).To(ContainSubstring("listener-::-8080"))

				// repeat to ensure any hashing doesn't short circuit returning the proxies
				reports, err = v.ValidateModifiedGvk(context.TODO(), v1.VirtualServiceGVK, snap.VirtualServices[0], false)
				Expect(err).NotTo(HaveOccurred())
				proxyReports = *(reports.ProxyReports)
				Expect(proxyReports).To(HaveLen(1))
				Expect(reports.GetProxies()).To(HaveLen(1))
				Expect(reports.GetProxies()[0]).To(ContainSubstring("listener-::-8080"))
			})
		})
		Context("no gateways for virtual service", func() {
			It("accepts the vs", func() {
				v.glooValidator = ValidateAccept
				snap := samples.SimpleGlooSnapshot(ns)
				snap.Gateways.Each(func(element *v1.Gateway) {
					switch gatewayType := element.GetGatewayType().(type) {
					case *v1.Gateway_HttpGateway:
						gatewayType.HttpGateway.VirtualServiceSelector = map[string]string{"nobody": "hastheselabels"}
					case *v1.Gateway_HybridGateway:
						for _, matchedGateway := range gatewayType.HybridGateway.GetMatchedGateways() {
							if httpGateway := matchedGateway.GetHttpGateway(); httpGateway != nil {
								httpGateway.VirtualServiceSelector = map[string]string{"nobody": "hastheselabels"}
							}
						}
					}
				})
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				reports, err := v.ValidateModifiedGvk(context.TODO(), v1.VirtualServiceGVK, snap.VirtualServices[0], false)
				Expect(err).NotTo(HaveOccurred())
				Expect(*(reports.ProxyReports)).To(HaveLen(1))
			})
		})
		Context("invalid selector expression for virtual service", func() {
			It("rejects the vs", func() {
				v.glooValidator = ValidateFail
				snap := samples.SimpleGlooSnapshot(ns)
				snap.Gateways.Each(func(element *v1.Gateway) {
					http, ok := element.GatewayType.(*v1.Gateway_HttpGateway)
					if !ok {
						return
					}
					http.HttpGateway.VirtualServiceExpressions = &v1.VirtualServiceSelectorExpressions{
						Expressions: []*v1.VirtualServiceSelectorExpressions_Expression{
							{
								Key:      "a",
								Operator: v1.VirtualServiceSelectorExpressions_Expression_Equals,
								Values:   []string{"b", "c"},
							},
						},
					}
				})
				err := v.Sync(context.TODO(), snap)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("expression is invalid"))
			})
		})
		Context("virtual service rejected", func() {
			It("rejects the vs", func() {
				badRoute := &v1.Route{
					Action: &v1.Route_DelegateAction{

						DelegateAction: &v1.DelegateAction{
							DelegationType: &v1.DelegateAction_Ref{
								Ref: &core.ResourceRef{
									Name:      "invalid",
									Namespace: "name",
								},
							},
						},
					},
				}

				// validate proxy should never be called
				v.glooValidator = nil
				snap := samples.SimpleGlooSnapshot(ns)
				vs := snap.VirtualServices[0].DeepCopyObject().(*v1.VirtualService)
				vs.VirtualHost.Routes = append(vs.VirtualHost.Routes, badRoute)

				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				reports, err := v.ValidateModifiedGvk(context.TODO(), v1.VirtualServiceGVK, vs, false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("could not render proxy"))
				Expect(*(reports.ProxyReports)).To(BeEmpty())
			})
		})
		Context("valid config gauge", func() {
			BeforeEach(func() {
				// reset the value before each test
				statsutils.Measure(context.TODO(), mValidConfig, -1)
			})
			It("returns 1 when there are no validation errors", func() {
				v.glooValidator = ValidateAccept

				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				_, err = v.ValidateModifiedGvk(context.TODO(), v1.VirtualServiceGVK, snap.VirtualServices[0], false)
				Expect(err).NotTo(HaveOccurred())

				rows, err := view.RetrieveData("validation.gateway.solo.io/valid_config")
				Expect(err).NotTo(HaveOccurred())
				Expect(rows).NotTo(BeEmpty())
				Expect(rows[0].Data.(*view.LastValueData).Value).To(BeEquivalentTo(1))
			})
			It("returns 0 when there are validation errors", func() {
				v.glooValidator = ValidateFail

				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				_, err = v.ValidateModifiedGvk(context.TODO(), v1.VirtualServiceGVK, snap.VirtualServices[0], false)
				Expect(err).To(HaveOccurred())

				rows, err := view.RetrieveData("validation.gateway.solo.io/valid_config")
				Expect(err).NotTo(HaveOccurred())
				Expect(rows).NotTo(BeEmpty())
				Expect(rows[0].Data.(*view.LastValueData).Value).To(BeEquivalentTo(0))
			})

			DescribeTable("validation with warnings", func(allowWarnings, disableValidationAgainstPreviousState bool, expectedMetric int) {
				v.allowWarnings = allowWarnings
				v.disableValidationAgainstPreviousState = disableValidationAgainstPreviousState
				v.glooValidator = ValidateWarn

				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.Background(), snap)
				Expect(err).NotTo(HaveOccurred())

				_, err = v.ValidateModifiedGvk(context.Background(), v1.VirtualServiceGVK, snap.VirtualServices[0], false)
				if expectedMetric == 1 {
					Expect(err).NotTo(HaveOccurred())
				} else {
					Expect(err).To(HaveOccurred())
				}

				rows, err := view.RetrieveData("validation.gateway.solo.io/valid_config")
				Expect(err).NotTo(HaveOccurred())
				Expect(rows).NotTo(BeEmpty())
				Expect(rows[0].Data.(*view.LastValueData).Value).To(BeEquivalentTo(expectedMetric))

			},
				// disableValidationAgainstPreviousState should not affect anything other than secrets
				Entry("allowWarnings=false, disableValidationAgainstPreviousState=true", false, true, 0),
				Entry("allowWarnings=true, disableValidationAgainstPreviousState=true", true, true, 1),
				Entry("allowWarnings=false, disableValidationAgainstPreviousState=false", false, false, 0),
				Entry("allowWarnings=true, disableValidationAgainstPreviousState=false", true, false, 1),
			)

			It("does not affect metrics when dryRun is true", func() {
				v.glooValidator = ValidateFail

				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				// Metric should be valid after successful Sync
				rows, err := view.RetrieveData("validation.gateway.solo.io/valid_config")
				Expect(err).NotTo(HaveOccurred())
				Expect(rows).NotTo(BeEmpty())
				Expect(rows[0].Data.(*view.LastValueData).Value).To(BeEquivalentTo(1))

				// Run a failed validation
				_, err = v.ValidateModifiedGvk(context.TODO(), v1.VirtualServiceGVK, snap.VirtualServices[0], true)
				Expect(err).To(HaveOccurred())

				// The metric should still be valid, since dryRun was true
				rows, err = view.RetrieveData("validation.gateway.solo.io/valid_config")
				Expect(err).NotTo(HaveOccurred())
				Expect(rows).NotTo(BeEmpty())
				Expect(rows[0].Data.(*view.LastValueData).Value).To(BeEquivalentTo(1))
			})
		})
		Context("dry-run", func() {
			It("accepts the vs and rejects the second", func() {
				v.glooValidator = ValidateAccept
				us := samples.SimpleUpstream()
				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				// create a virtual service to validate, should pass validation as a prior one should
				// already be in the validation snapshot cache with a different domain
				samples.AddVsToSnap(snap, us.GetMetadata().Ref(), "ns")
				vs2 := &v1.VirtualService{}
				snap.VirtualServices[1].DeepCopyInto(vs2)
				vs2.Metadata.Name = "vs2"

				reports, err := v.ValidateModifiedGvk(context.TODO(), v1.VirtualServiceGVK, vs2, false)
				Expect(err).NotTo(HaveOccurred())
				Expect(*(reports.ProxyReports)).To(HaveLen(1))

				// create another virtual service to validate, should fail validation as a prior one should
				// already be in the validation snapshot cache with the same domain (as dry-run before was false)
				vs3 := &v1.VirtualService{}
				snap.VirtualServices[1].DeepCopyInto(vs3)
				vs3.Metadata.Name = "vs3"

				reports, err = v.ValidateModifiedGvk(context.TODO(), v1.VirtualServiceGVK, vs3, false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("could not render proxy"))
				Expect(err.Error()).To(ContainSubstring("domain conflict: the following"))
				Expect(*(reports.ProxyReports)).To(BeEmpty())
			})

			It("accepts the vs and accepts the second because of dry-run", func() {
				v.glooValidator = ValidateAccept
				us := samples.SimpleUpstream()
				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				samples.AddVsToSnap(snap, us.GetMetadata().Ref(), "ns")
				// create a virtual service to validate, should pass validation as a prior one should
				// already be in the validation snapshot cache with a different domain
				vs2 := &v1.VirtualService{}
				snap.VirtualServices[1].DeepCopyInto(vs2)
				vs2.Metadata.Name = "vs2"

				reports, err := v.ValidateModifiedGvk(context.TODO(), v1.VirtualServiceGVK, vs2, true)
				Expect(err).NotTo(HaveOccurred())
				Expect(*(reports.ProxyReports)).To(HaveLen(1))

				// create another virtual service to validate, should pass validation as a prior one should not
				// already be in the validation snapshot cache (as dry-run was true)
				vs3 := &v1.VirtualService{}
				snap.VirtualServices[1].DeepCopyInto(vs3)
				vs3.Metadata.Name = "vs3"

				reports, err = v.ValidateModifiedGvk(context.TODO(), v1.VirtualServiceGVK, vs3, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(*(reports.ProxyReports)).To(HaveLen(1))
			})
		})
	})

	Context("delete a virtual service", func() {
		Context("has parent gateways", func() {
			It("rejects deletion", func() {
				v.glooValidator = ValidateAccept
				snap := samples.SimpleGlooSnapshot(ns)
				ref := snap.VirtualServices[0].Metadata.Ref()
				snap.Gateways.Each(func(element *v1.Gateway) {
					switch gatewayType := element.GetGatewayType().(type) {
					case *v1.Gateway_HttpGateway:
						gatewayType.HttpGateway.VirtualServices = []*core.ResourceRef{ref}
					case *v1.Gateway_HybridGateway:
						for _, matchedGateway := range gatewayType.HybridGateway.GetMatchedGateways() {
							if httpGateway := matchedGateway.GetHttpGateway(); httpGateway != nil {
								httpGateway.VirtualServices = []*core.ResourceRef{ref}
							}
						}
					}
					http, ok := element.GatewayType.(*v1.Gateway_HttpGateway)
					if !ok {
						return
					}
					http.HttpGateway.VirtualServices = []*core.ResourceRef{ref}
				})
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				err = v.ValidateDeletedGvk(context.TODO(), v1.VirtualServiceGVK, snap.VirtualServices[0], false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("invalid virtual service ref name:\"%s\"", ref.Name)))
				Expect(err.Error()).To(ContainSubstring("hybrid gateway does not have any populated matched gateways"))
			})
		})
		Context("has no parent gateways", func() {
			It("unable to delete vs, because it is assigned to hybrid gateway", func() {
				v.glooValidator = ValidateAccept
				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				ref := snap.VirtualServices[0].Metadata.Ref()
				err = v.ValidateDeletedGvk(context.TODO(), v1.VirtualServiceGVK, snap.VirtualServices[0], false)
				Expect(err).To(HaveOccurred())

				_, err = v.latestSnapshot.VirtualServices.Find(ref.Strings())
				Expect(err).ToNot(HaveOccurred())
			})

			It("deletes safely", func() {
				v.glooValidator = ValidateAccept
				snap := samples.SimpleGlooSnapshot(ns)
				newVS := samples.SimpleVS("newnamespace", "newname", "mydomain", snap.Upstreams[0].Metadata.Ref())
				err := snap.UpsertToResourceList(newVS)
				Expect(err).NotTo(HaveOccurred())
				err = v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				ref := newVS.Metadata.Ref()
				err = v.ValidateDeletedGvk(context.TODO(), v1.VirtualServiceGVK, newVS, false)
				Expect(err).ToNot(HaveOccurred())

				_, err = v.latestSnapshot.VirtualServices.Find(ref.Strings())
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Context("validating a gateway", func() {
		Context("proxy validation returns error", func() {
			It("rejects the gw", func() {
				v.glooValidator = ValidateFail
				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				// change something to change the hash
				snap.Gateways[0].Metadata.Labels = map[string]string{"change": "my mind"}

				reports, err := v.ValidateModifiedGvk(context.TODO(), v1.GatewayGVK, snap.Gateways[0], false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(glooValidationError))
				Expect(*(reports.ProxyReports)).To(HaveLen(1))
			})
		})
		Context("proxy validation accepted", func() {
			It("accepts the gw", func() {
				v.glooValidator = ValidateAccept
				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				reports, err := v.ValidateModifiedGvk(context.TODO(), v1.GatewayGVK, snap.Gateways[0], false)
				Expect(err).NotTo(HaveOccurred())
				Expect(*(reports.ProxyReports)).To(HaveLen(1))
			})

			It("accepts the gateway and returns proxies each time", func() {
				v.glooValidator = ValidateAccept
				snap := samples.GlooSnapshotWithDelegates(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				reports, err := v.ValidateModifiedGvk(context.TODO(), v1.GatewayGVK, snap.Gateways[0], false)
				Expect(err).NotTo(HaveOccurred())
				proxyReports := *(reports.ProxyReports)
				Expect(proxyReports).To(HaveLen(1))
				Expect(reports.GetProxies()).To(HaveLen(1))
				Expect(reports.GetProxies()[0]).To(ContainSubstring("listener-::-8080"))

				// repeat to ensure any hashing doesn't short circuit returning the proxies
				reports, err = v.ValidateModifiedGvk(context.TODO(), v1.GatewayGVK, snap.Gateways[0], false)
				Expect(err).NotTo(HaveOccurred())
				proxyReports = *(reports.ProxyReports)
				Expect(proxyReports).To(HaveLen(1))
				Expect(reports.GetProxies()).To(HaveLen(1))
				Expect(reports.GetProxies()[0]).To(ContainSubstring("listener-::-8080"))
			})
		})
		Context("gw rejected", func() {
			It("rejects the gw", func() {
				badRef := &core.ResourceRef{}

				// validate proxy should never be called
				v.glooValidator = nil
				snap := samples.SimpleGlooSnapshot(ns)
				gw := snap.Gateways[0].DeepCopyObject().(*v1.Gateway)

				gw.GatewayType.(*v1.Gateway_HttpGateway).HttpGateway.VirtualServices = append(gw.GatewayType.(*v1.Gateway_HttpGateway).HttpGateway.VirtualServices, badRef)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				reports, err := v.ValidateModifiedGvk(context.TODO(), v1.GatewayGVK, gw, false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("could not render proxy"))
				Expect(*(reports.ProxyReports)).To(BeEmpty())
			})
		})

	})

	Context("validating a list of virtual services", func() {

		toUnstructuredList := func(vss ...*v1.VirtualService) *unstructured.UnstructuredList {

			var objs []unstructured.Unstructured
			for _, vs := range vss {
				kubeRes, _ := v1.VirtualServiceCrd.KubeResource(vs)
				bytes, err := json.Marshal(kubeRes)
				Expect(err).ToNot(HaveOccurred())
				mapFromVs := map[string]interface{}{}

				// NOTE: This is not the default golang yaml.Unmarshal, because that implementation
				// does not unmarshal into a map[string]interface{}; it unmarshals the file into a map[interface{}]interface{}
				// https://github.com/go-yaml/yaml/issues/139
				err = k8syamlutil.Unmarshal(bytes, &mapFromVs)
				Expect(err).ToNot(HaveOccurred())

				obj := unstructured.Unstructured{Object: mapFromVs}
				objs = append(objs, obj)
			}

			return &unstructured.UnstructuredList{
				Object: map[string]interface{}{
					"kind":    "List",
					"version": "v1",
				},
				Items: objs,
			}
		}

		Context("proxy validation returns error", func() {
			It("rejects the vs list", func() {
				v.glooValidator = ValidateFail
				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				// change something to change the hash
				snap.VirtualServices[0].Metadata.Labels = map[string]string{"change": "my mind"}
				vsList := toUnstructuredList(snap.VirtualServices[0])

				reports, err := v.ValidateList(context.TODO(), vsList, false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(glooValidationError))
				Expect(*(reports.ProxyReports)).To(HaveLen(1))

			})
		})

		Context("proxy validation accepted", func() {

			It("accepts the vs list", func() {
				v.glooValidator = ValidateAccept
				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				reports, merr := v.ValidateList(context.TODO(), toUnstructuredList(snap.VirtualServices[0]), false)
				Expect(merr.ErrorOrNil()).NotTo(HaveOccurred())
				Expect(*(reports.ProxyReports)).To(HaveLen(1))
			})

			It("accepts the multi vs list", func() {
				v.glooValidator = ValidateAccept
				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				vs1 := &v1.VirtualService{}
				snap.VirtualServices[0].DeepCopyInto(vs1)
				vs1.Metadata.Name = "vs1"
				vs1.VirtualHost.Domains = []string{"example.vs1.com"}
				vs2 := &v1.VirtualService{}
				snap.VirtualServices[0].DeepCopyInto(vs2)
				vs2.Metadata.Name = "vs2"
				vs2.VirtualHost.Domains = []string{"example.vs2.com"}

				reports, merr := v.ValidateList(context.TODO(), toUnstructuredList(vs1, vs2), false)
				Expect(merr.ErrorOrNil()).NotTo(HaveOccurred())
				Expect(*(reports.ProxyReports)).To(HaveLen(2))
			})

			It("rejects the multi vs list with overlapping domains", func() {
				v.glooValidator = ValidateAccept
				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				vs1 := &v1.VirtualService{}
				snap.VirtualServices[0].DeepCopyInto(vs1)
				vs1.Metadata.Name = "vs1"

				vs2 := &v1.VirtualService{}
				snap.VirtualServices[0].DeepCopyInto(vs2)
				vs2.Metadata.Name = "vs2"

				reports, err := v.ValidateList(context.TODO(), toUnstructuredList(vs1, vs2), false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("could not render proxy"))
				Expect(err.Error()).To(ContainSubstring("domain conflict: the following"))
				Expect(*(reports.ProxyReports)).To(BeEmpty())
			})

			It("accepts the vs list and returns proxies each time", func() {
				v.glooValidator = ValidateAccept
				snap := samples.GlooSnapshotWithDelegates(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				reports, merr := v.ValidateList(context.TODO(), toUnstructuredList(snap.VirtualServices[0]), false)
				Expect(merr.ErrorOrNil()).NotTo(HaveOccurred())
				proxyReports := *(reports.ProxyReports)
				Expect(proxyReports).To(HaveLen(1))
				Expect(reports.GetProxies()).To(HaveLen(1))
				Expect(reports.GetProxies()[0]).To(ContainSubstring("listener-::-8080"))

				// repeat to ensure any hashing doesn't short circuit returning the proxies
				reports, merr = v.ValidateList(context.TODO(), toUnstructuredList(snap.VirtualServices[0]), false)
				Expect(merr.ErrorOrNil()).NotTo(HaveOccurred())
				proxyReports = *(reports.ProxyReports)
				Expect(proxyReports).To(HaveLen(1))
				Expect(reports.GetProxies()).To(HaveLen(1))
				Expect(reports.GetProxies()[0]).To(ContainSubstring("listener-::-8080"))
			})
		})

		Context("unmarshal errors", func() {
			It("doesn't mask other errors when there's an unmarshal error in a list", func() {
				v.glooValidator = ValidateAccept
				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				ul := &unstructured.UnstructuredList{}
				jsonBytes, err := os.ReadFile("fixtures/unmarshal-err.json")
				Expect(err).ToNot(HaveOccurred())
				err = ul.UnmarshalJSON(jsonBytes)
				Expect(err).ToNot(HaveOccurred())
				reports, merr := v.ValidateList(context.TODO(), ul, false)
				Expect(merr).To(HaveOccurred())
				Expect(merr.Errors).To(HaveLen(3))
				Expect(merr.Errors[0]).To(MatchError(ContainSubstring("route table gloo-system.i-dont-exist-rt missing")))
				Expect(merr.Errors[1]).To(MatchError(ContainSubstring("virtual service [gloo-system.invalid-vs-2] does not specify a virtual host")))
				Expect(merr.Errors[2]).To(MatchError(ContainSubstring("parsing resource from crd spec testproxy1-rt in namespace gloo-system into *v1.RouteTable")))
				Expect(merr.Errors[2]).To(MatchError(ContainSubstring("unknown field \"matcherss\" in gateway.solo.io.Route")))
				Expect(*(reports.ProxyReports)).To(BeEmpty())

			})
		})

		Context("virtual service list rejected", func() {
			It("rejects the vs list", func() {
				badRoute := &v1.Route{
					Action: &v1.Route_DelegateAction{

						DelegateAction: &v1.DelegateAction{
							DelegationType: &v1.DelegateAction_Ref{
								Ref: &core.ResourceRef{
									Name:      "invalid",
									Namespace: "name",
								},
							},
						},
					},
				}

				// validate proxy should never be called
				v.glooValidator = nil
				snap := samples.SimpleGlooSnapshot(ns)
				vs := snap.VirtualServices[0].DeepCopyObject().(*v1.VirtualService)
				vs.VirtualHost.Routes = append(vs.VirtualHost.Routes, badRoute)
				vsList := toUnstructuredList(vs)

				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())
				reports, err := v.ValidateList(context.TODO(), vsList, false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("could not render proxy"))
				Expect(*(reports.ProxyReports)).To(BeEmpty())
			})
		})

		Context("dry-run", func() {
			It("accepts the vs and rejects the second", func() {
				v.glooValidator = ValidateAccept
				us := samples.SimpleUpstream()
				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				// create a virtual service to validate, should pass validation as a prior one should
				// already be in the validation snapshot cache with a different domain
				samples.AddVsToSnap(snap, us.GetMetadata().Ref(), "ns")

				vs2 := &v1.VirtualService{}
				snap.VirtualServices[1].DeepCopyInto(vs2)
				vs2.Metadata.Name = "vs2"

				reports, merr := v.ValidateList(context.TODO(), toUnstructuredList(vs2), false)
				Expect(merr.ErrorOrNil()).NotTo(HaveOccurred())
				Expect(*(reports.ProxyReports)).To(HaveLen(1))

				// create another virtual service to validate, should fail validation as a prior one should
				// already be in the validation snapshot cache with the same domain (as dry-run before was false)
				vs3 := &v1.VirtualService{}
				snap.VirtualServices[1].DeepCopyInto(vs3)
				vs3.Metadata.Name = "vs3"

				reports, merr = v.ValidateList(context.TODO(), toUnstructuredList(vs3), false)
				Expect(merr.ErrorOrNil()).To(HaveOccurred())
				Expect(merr.ErrorOrNil().Error()).To(ContainSubstring("could not render proxy"))
				Expect(merr.ErrorOrNil().Error()).To(ContainSubstring("domain conflict: the following"))
				Expect(*(reports.ProxyReports)).To(BeEmpty())
			})

			It("accepts the vs and accepts the second because of dry-run", func() {
				v.glooValidator = ValidateAccept
				us := samples.SimpleUpstream()
				snap := samples.SimpleGlooSnapshot(ns)
				err := v.Sync(context.TODO(), snap)
				Expect(err).NotTo(HaveOccurred())

				samples.AddVsToSnap(snap, us.GetMetadata().Ref(), "ns")
				// create a virtual service to validate, should pass validation as a prior one should
				// already be in the validation snapshot cache with a different domain
				vs2 := &v1.VirtualService{}
				snap.VirtualServices[1].DeepCopyInto(vs2)
				vs2.Metadata.Name = "vs2"

				reports, merr := v.ValidateList(context.TODO(), toUnstructuredList(vs2), true)
				Expect(merr.ErrorOrNil()).NotTo(HaveOccurred())
				Expect(*(reports.ProxyReports)).To(HaveLen(1))

				// create another virtual service to validate, should pass validation as a prior one should not
				// already be in the validation snapshot cache (as dry-run was true)
				vs3 := &v1.VirtualService{}
				snap.VirtualServices[1].DeepCopyInto(vs3)
				vs3.Metadata.Name = "vs3"

				reports, merr = v.ValidateList(context.TODO(), toUnstructuredList(vs3), true)
				Expect(merr.ErrorOrNil()).ToNot(HaveOccurred())
				Expect(*(reports.ProxyReports)).To(HaveLen(1))
			})

		})

	})

	Context("validating concurrent scenario", func() {

		var (
			resultMap       sync.Map
			numberOfWorkers int
		)

		BeforeEach(func() {
			resultMap = sync.Map{}
			numberOfWorkers = 100
		})

		validateModifiedGvkWorker := func(vsToDuplicate *v1.VirtualService, name string, wg *sync.WaitGroup) {
			defer GinkgoRecover()
			// duplicate the vs with a different name
			workerVirtualService := &v1.VirtualService{}
			vsToDuplicate.DeepCopyInto(workerVirtualService)
			workerVirtualService.Metadata.Name = "vs2-" + name
			_, err := v.ValidateModifiedGvk(context.TODO(), v1.VirtualServiceGVK, workerVirtualService, false)

			if err != nil {
				// worker errors are stored in the resultMap
				resultMap.Store(name, err.Error())
			}
			wg.Done()
		}

		It("accepts only 1 vs when multiple are written concurrently", func() {
			v.glooValidator = ValidateAccept
			us := samples.SimpleUpstream()
			snap := samples.SimpleGlooSnapshot(ns)
			err := v.Sync(context.TODO(), snap)
			Expect(err).NotTo(HaveOccurred())

			samples.AddVsToSnap(snap, us.GetMetadata().Ref(), "ns")
			vsToDuplicate := snap.VirtualServices[1]

			// start workers
			wg := sync.WaitGroup{}
			wg.Add(numberOfWorkers)
			for i := range numberOfWorkers {
				workerName := fmt.Sprintf("worker #%d", i)
				// validateModifiedGvkWorker handles its own GinkgoRecover and
				// stores errors in resultMap where we aggregate them later
				go validateModifiedGvkWorker(vsToDuplicate, workerName, &wg)
			}

			// wait for all workers to complete
			wg.Wait()

			// aggregate the error messages from all the workers
			var errMessages []string
			resultMap.Range(func(name, value interface{}) bool {
				errMessages = append(errMessages, value.(string))
				return true
			})

			// Expect 1 worker to have successfully completed and all others to have failed
			Expect(errMessages).To(HaveLen(numberOfWorkers - 1))
		})

	})
})

// ValidationAddErrorsAndWarnings is a helper function to add errors and warnings to a proxy
func ValidationAddErrorsAndWarnings(proxy *gloov1.Proxy, validationReports []*gloovalidation.GlooValidationReport, errMessages []string, warnMessages []string) ([]*gloovalidation.GlooValidationReport, error) {
	var proxies []*gloov1.Proxy
	if proxy != nil {
		proxies = []*gloov1.Proxy{proxy}
	} else {
		proxies = samples.SimpleGlooSnapshot("gloo-system").Proxies
	}

	for _, proxy := range proxies {
		proxyReport := validationutils.MakeReport(proxy)

		for _, errMessage := range errMessages {
			validationutils.AppendListenerError(proxyReport.ListenerReports[0], validation.ListenerReport_Error_SSLConfigError, errMessage)
		}

		for _, warnMessage := range warnMessages {
			validationutils.AppendRouteWarning(proxyReport.ListenerReports[0].GetHttpListenerReport().GetVirtualHostReports()[0].GetRouteReports()[0], validation.RouteReport_Warning_InvalidDestinationWarning, warnMessage)
		}

		validationReports = append(validationReports, &gloovalidation.GlooValidationReport{
			Proxy:       proxy,
			ProxyReport: proxyReport,
		})
	}

	return validationReports, nil
}

func ValidationWithResourceReports(reportGenerator func() reporter.ResourceReports) validationFunc {
	return func(ctx context.Context, proxy *gloov1.Proxy, resource resources.Resource, shouldDelete bool) ([]*gloovalidation.GlooValidationReport, error) {
		var validationReports []*gloovalidation.GlooValidationReport
		var proxies []*gloov1.Proxy
		if proxy != nil {
			proxies = []*gloov1.Proxy{proxy}
		} else {
			proxies = samples.SimpleGlooSnapshot("gloo-system").Proxies
		}

		for _, proxy := range proxies {
			validationReports = append(validationReports, &gloovalidation.GlooValidationReport{
				Proxy:           proxy,
				ResourceReports: reportGenerator(),
			})
		}

		return validationReports, nil
	}
}

const (
	warnString  = "this is a warning"
	errString   = "this is an error"
	warnStringF = "this is a warning %d"
	errStringF  = "this is an error %d"
)

// Validation functions here. There return reports that are processed by the validator
// In order to test the secret valdiation logic, there needs to be a way to change the error message and/or warning.
// These validation functions have the word "Change" in their names and will return a unique error/warning message each time they are called.
func ValidateFail(ctx context.Context, proxy *gloov1.Proxy, resource resources.Resource, shouldDelete bool) ([]*gloovalidation.GlooValidationReport, error) {
	return ValidationAddErrorsAndWarnings(proxy, nil, []string{errString}, []string{})
}

func ValidateAccept(ctx context.Context, proxy *gloov1.Proxy, resource resources.Resource, shouldDelete bool) ([]*gloovalidation.GlooValidationReport, error) {
	return ValidationAddErrorsAndWarnings(proxy, nil, []string{}, []string{})
}

func ValidateWarn(ctx context.Context, proxy *gloov1.Proxy, resource resources.Resource, shouldDelete bool) ([]*gloovalidation.GlooValidationReport, error) {
	return ValidationAddErrorsAndWarnings(proxy, nil, []string{}, []string{warnString})
}

// Integers appended to errors/warnings to create different error/warning messages for testing secret deletion
var validateChangeErrCnt = 0
var validateChangeWarningCnt = 0

func ValidateFailChangeError(ctx context.Context, proxy *gloov1.Proxy, resource resources.Resource, shouldDelete bool) ([]*gloovalidation.GlooValidationReport, error) {
	validateChangeErrCnt++
	return ValidationAddErrorsAndWarnings(proxy, nil, []string{fmt.Sprintf(errStringF, validateChangeErrCnt)}, []string{})
}

func ValidateChangeWarning(ctx context.Context, proxy *gloov1.Proxy, resource resources.Resource, shouldDelete bool) ([]*gloovalidation.GlooValidationReport, error) {
	validateChangeWarningCnt++
	return ValidationAddErrorsAndWarnings(proxy, nil, []string{}, []string{fmt.Sprintf(warnStringF, validateChangeWarningCnt)})
}

func ValidationErrorAndWarn(ctx context.Context, proxy *gloov1.Proxy, resource resources.Resource, shouldDelete bool) ([]*gloovalidation.GlooValidationReport, error) {
	return ValidationAddErrorsAndWarnings(proxy, nil, []string{errString}, []string{warnString})
}

func ValidateChangeWarningAndChangeError(ctx context.Context, proxy *gloov1.Proxy, resource resources.Resource, shouldDelete bool) ([]*gloovalidation.GlooValidationReport, error) {
	validateChangeWarningCnt++
	validateChangeErrCnt++
	return ValidationAddErrorsAndWarnings(proxy, nil, []string{fmt.Sprintf(errStringF, validateChangeErrCnt)}, []string{fmt.Sprintf(warnStringF, validateChangeWarningCnt)})
}

func ValidateChangeWarningAndSameError(ctx context.Context, proxy *gloov1.Proxy, resource resources.Resource, shouldDelete bool) ([]*gloovalidation.GlooValidationReport, error) {
	validateChangeWarningCnt++
	return ValidationAddErrorsAndWarnings(proxy, nil, []string{errString}, []string{fmt.Sprintf(warnStringF, validateChangeWarningCnt)})
}

func ValidateSameWarningAndChangeError(ctx context.Context, proxy *gloov1.Proxy, resource resources.Resource, shouldDelete bool) ([]*gloovalidation.GlooValidationReport, error) {
	validateChangeErrCnt++
	return ValidationAddErrorsAndWarnings(proxy, nil, []string{fmt.Sprintf(errStringF, validateChangeErrCnt)}, []string{warnString})
}

func ValidateNoGlooSync(ctx context.Context, proxy *gloov1.Proxy, resource resources.Resource, shouldDelete bool) ([]*gloovalidation.GlooValidationReport, error) {
	return nil, gloovalidation.SyncNotCalledError
}

func ValidateResponseLengthError(ctx context.Context, proxy *gloov1.Proxy, resource resources.Resource, shouldDelete bool) ([]*gloovalidation.GlooValidationReport, error) {
	return nil, GlooValidationResponseLengthError{}
}

// type Translator interface {
// 	Translate(ctx context.Context, proxyName string, snap *gloov1snap.ApiSnapshot, filteredGateways v1.GatewayList) (*gloov1.Proxy, reporter.ResourceReports)
// }

// MockTranslator is a mock translator that can be used to test the validator
type MockTranslator struct {
	Translator      translator.Translator // Would call it `T`, but that would be confusing
	ReportGenerator func() reporter.ResourceReports
}

// Translate uses te real translator to generate the snapshot, then overrides the reports with the output of ReportGenerator function
func (m *MockTranslator) Translate(ctx context.Context, proxyName string, snap *gloov1snap.ApiSnapshot, filteredGateways v1.GatewayList) (*gloov1.Proxy, reporter.ResourceReports) {
	proxy, _ := m.Translator.Translate(ctx, proxyName, snap, filteredGateways)
	return proxy, m.ReportGenerator()
}

func generateTranslationReports(rep reporter.Report) reporter.ResourceReports {
	res := &skmocks.MockResource{
		Metadata: &core.Metadata{
			Name:      "r0",
			Namespace: "ns",
		},
	}

	return reporter.ResourceReports{
		res: rep,
	}
}

func generateErrorAndWarn() reporter.ResourceReports {
	return generateTranslationReports(reporter.Report{
		Errors: &multierror.Error{Errors: []error{fmt.Errorf(errString)}}, Warnings: []string{warnString},
	})
}

func generateError() reporter.ResourceReports {
	return generateTranslationReports(reporter.Report{
		Errors: &multierror.Error{Errors: []error{fmt.Errorf(errString)}},
	})
}

func generateWarn() reporter.ResourceReports {
	return generateTranslationReports(reporter.Report{
		Warnings: []string{warnString},
	})
}

func generateNone() reporter.ResourceReports {
	return generateTranslationReports(reporter.Report{})
}

func generateChangeError() reporter.ResourceReports {
	validateChangeErrCnt++
	return generateTranslationReports(reporter.Report{
		Errors: &multierror.Error{Errors: []error{fmt.Errorf(fmt.Sprintf(errStringF, validateChangeErrCnt))}},
	})
}

func generateChangeWarn() reporter.ResourceReports {
	validateChangeWarningCnt++
	return generateTranslationReports(reporter.Report{
		Warnings: []string{fmt.Sprintf(warnStringF, validateChangeWarningCnt)},
	})
}

func generateChangeWarnSameError() reporter.ResourceReports {
	validateChangeWarningCnt++
	return generateTranslationReports(reporter.Report{
		Errors: &multierror.Error{Errors: []error{fmt.Errorf(errString)}}, Warnings: []string{fmt.Sprintf(warnStringF, validateChangeWarningCnt)},
	})
}

func generateSameWarnChangeError() reporter.ResourceReports {
	validateChangeErrCnt++
	return generateTranslationReports(reporter.Report{
		Errors: &multierror.Error{Errors: []error{fmt.Errorf(fmt.Sprintf(errStringF, validateChangeErrCnt))}}, Warnings: []string{warnString},
	})
}

func generateChangeWarnChangeError() reporter.ResourceReports {
	validateChangeErrCnt++
	validateChangeWarningCnt++
	return generateTranslationReports(reporter.Report{
		Errors: &multierror.Error{Errors: []error{fmt.Errorf(fmt.Sprintf("r0err1-%d", validateChangeErrCnt))}}, Warnings: []string{fmt.Sprintf("r0warn1-%d", validateChangeWarningCnt)},
	})
}

type MockExtensionValidator struct {
	ReportGenerator func() reporter.ResourceReports
}

func (m *MockExtensionValidator) Validate(ctx context.Context, snapshot *gloov1snap.ApiSnapshot) reporter.ResourceReports {
	return m.ReportGenerator()
}
