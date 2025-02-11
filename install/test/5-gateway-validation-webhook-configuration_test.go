//go:build ignore

package test

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/onsi/gomega/types"
	v1 "k8s.io/api/admissionregistration/v1"

	glootestutils "github.com/kgateway-dev/kgateway/v2/test/testutils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/go-utils/stringutils"
	"github.com/solo-io/k8s-utils/installutils/kuberesource"
	. "github.com/solo-io/k8s-utils/manifesttestutils"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	gloostringutils "github.com/kgateway-dev/kgateway/v2/pkg/utils/stringutils"
)

const (
	validatingWebhookConfigurationKind = "ValidatingWebhookConfiguration"
)

var _ = Describe("WebhookValidationConfiguration helm test", func() {
	var allTests = func(rendererTestCase renderTestCase) {

		var (
			testManifest TestManifest
			//expectedChart *unstructured.Unstructured
		)
		prepareMakefile := func(namespace string, values glootestutils.HelmValues) {
			tm, err := rendererTestCase.renderer.RenderManifest(namespace, values)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to render manifest")
			testManifest = tm
		}

		DescribeTable("Can remove DELETEs from webhook rules", func(resources []string, expectedRemoved int) {
			timeoutSeconds := 5

			// Count the "DELETES" as a sanity check.
			expectedDeletes := 6 - expectedRemoved
			expectedChart := generateExpectedChart(expectedChartArgs{
				timeoutSeconds:  timeoutSeconds,
				skipDeletes:     resources,
				expectedDeletes: expectedDeletes,
			})

			prepareMakefile(namespace, glootestutils.HelmValues{
				ValuesArgs: []string{
					fmt.Sprintf(`gateway.validation.webhook.timeoutSeconds=%d`, timeoutSeconds),
					`gateway.validation.webhook.skipDeleteValidationResources={` + strings.Join(resources, ",") + `}`,
				},
			})
			testManifest.ExpectUnstructured(expectedChart.GetKind(), expectedChart.GetNamespace(), expectedChart.GetName()).To(BeEquivalentTo(expectedChart))
		},
			Entry("virtualservices", []string{"virtualservices"}, 1),
			Entry("routetables", []string{"routetables"}, 1),
			Entry("gateways", []string{"gateways"}, 0),
			Entry("upstreams", []string{"upstreams"}, 1),
			Entry("secrets", []string{"secrets"}, 1),
			Entry("namespaces", []string{"namespaces"}, 1),
			Entry("ratelimitconfigs", []string{"ratelimitconfigs"}, 1),
			Entry("mutiple", []string{"virtualservices", "routetables", "secrets"}, 3),
			Entry("mutiple (with removal)", []string{"ratelimitconfigs", "secrets"}, 2),
			Entry("all", []string{"*"}, 6),
			Entry("empty", []string{}, 0),
		)

		It("Can set gloo failurePolicy", func() {
			timeoutSeconds := 5
			expectedChart := generateExpectedChart(expectedChartArgs{
				timeoutSeconds:    timeoutSeconds,
				glooFailurePolicy: "Fail",
				expectedDeletes:   6,
			})

			prepareMakefile(namespace, glootestutils.HelmValues{
				ValuesArgs: []string{
					fmt.Sprintf(`gateway.validation.webhook.timeoutSeconds=%d`, timeoutSeconds),
					`gateway.validation.failurePolicy=Fail`,
				},
			})
			testManifest.ExpectUnstructured(expectedChart.GetKind(), expectedChart.GetNamespace(), expectedChart.GetName()).To(BeEquivalentTo(expectedChart))
		})

		It("Can set kube failurePolicy", func() {
			timeoutSeconds := 5
			expectedChart := generateExpectedChart(expectedChartArgs{
				timeoutSeconds:    timeoutSeconds,
				kubeFailurePolicy: "Fail",
				expectedDeletes:   6,
			})

			prepareMakefile(namespace, glootestutils.HelmValues{
				ValuesArgs: []string{
					fmt.Sprintf(`gateway.validation.webhook.timeoutSeconds=%d`, timeoutSeconds),
					`gateway.validation.kubeCoreFailurePolicy=Fail`,
				},
			})
			testManifest.ExpectUnstructured(expectedChart.GetKind(), expectedChart.GetNamespace(), expectedChart.GetName()).To(BeEquivalentTo(expectedChart))
		})

		It("Kube Webhook is not rendered if secrets and resources are skipped", func() {
			prepareMakefile(namespace, glootestutils.HelmValues{
				ValuesArgs: []string{
					`gateway.validation.webhook.skipDeleteValidationResources={secrets,namespaces}`,
				},
			})

			// assert that the kube webhook is not rendered
			testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
				return resource.GetKind() == "ValidatingWebhookConfiguration" && resource.GetName() == "gloo-gateway-validation-webhook-"+namespace
			}).ExpectAll(func(webhook *unstructured.Unstructured) {
				webhookObject, err := kuberesource.ConvertUnstructured(webhook)

				ExpectWithOffset(1, err).NotTo(HaveOccurred(), fmt.Sprintf("Webhook %+v should be able to convert from unstructured", webhook))
				structuredDeployment, ok := webhookObject.(*admissionregistrationv1.ValidatingWebhookConfiguration)
				ExpectWithOffset(1, ok).To(BeTrue(), fmt.Sprintf("Webhook %+v should be able to cast to a structured deployment", webhook))

				ExpectWithOffset(1, structuredDeployment.Webhooks).To(HaveLen(1), fmt.Sprintf("Only one webhook should be present on deployment %+v", structuredDeployment))
				ExpectWithOffset(1, structuredDeployment.Webhooks[0].Name).To(Equal("gloo."+namespace+".svc"), fmt.Sprintf("Webhook name should be 'gloo.%s.svc' on deployment %+v", namespace, structuredDeployment))
			})
		})

		Context("enablePolicyApi", func() {

			// containPolicyApiOperation returns a GomegaMatcher which will assert that a provided ValidatingWebhookConfiguration
			// contains the Rule that includes the Policy APIs
			containPolicyApiOperation := func() types.GomegaMatcher {
				policyApiOperation := v1.RuleWithOperations{
					Operations: []v1.OperationType{
						v1.Create,
						v1.Update,
					},
					Rule: v1.Rule{
						APIGroups:   []string{"gateway.solo.io"},
						APIVersions: []string{"v1"},
						Resources:   []string{"routeoptions", "virtualhostoptions"},
					},
				}
				return WithTransform(func(config *v1.ValidatingWebhookConfiguration) []v1.RuleWithOperations {
					if config == nil {
						return nil
					}
					return config.Webhooks[0].Rules
				}, ContainElement(policyApiOperation))
			}

			type enablePolicyApiCase struct {
				enableKubeGatewayApi         *wrappers.BoolValue
				enablePolicyApi              *wrappers.BoolValue
				expectedWebhookConfiguration types.GomegaMatcher
			}

			DescribeTable("respects helm values",
				func(testCase enablePolicyApiCase) {
					var valuesArgs []string
					if testCase.enableKubeGatewayApi != nil {
						valuesArgs = append(valuesArgs, fmt.Sprintf(`kubeGateway.enabled=%t`, testCase.enableKubeGatewayApi.GetValue()))
					}
					if testCase.enablePolicyApi != nil {
						valuesArgs = append(valuesArgs, fmt.Sprintf(`gateway.validation.webhook.enablePolicyApi=%t`, testCase.enablePolicyApi.GetValue()))
					}

					prepareMakefile(namespace, glootestutils.HelmValues{
						ValuesArgs: valuesArgs,
					})

					testManifest.ExpectUnstructured(
						validatingWebhookConfigurationKind,
						"", // ValidatingWebhookConfiguration is cluster-scoped
						fmt.Sprintf("gloo-gateway-validation-webhook-%s", namespace),
					).To(WithTransform(func(unstructuredVwc *unstructured.Unstructured) *v1.ValidatingWebhookConfiguration {
						// convert the unstructured validating webhook configuration to a structured object
						if unstructuredVwc == nil {
							return nil
						}

						rawJson, err := json.Marshal(unstructuredVwc.Object)
						if err != nil {
							return nil
						}

						var structuredVwc *v1.ValidatingWebhookConfiguration
						err = json.Unmarshal(rawJson, &structuredVwc)
						if err != nil {
							return nil
						}

						return structuredVwc
					}, testCase.expectedWebhookConfiguration))
				},
				Entry("unset", enablePolicyApiCase{
					enableKubeGatewayApi:         nil,
					enablePolicyApi:              nil,
					expectedWebhookConfiguration: Not(containPolicyApiOperation()),
				}),
				Entry("enableKubeGatewayApi=false,", enablePolicyApiCase{
					enableKubeGatewayApi:         &wrappers.BoolValue{Value: false},
					enablePolicyApi:              nil,
					expectedWebhookConfiguration: Not(containPolicyApiOperation()),
				}),
				Entry("enableKubeGatewayApi=true,", enablePolicyApiCase{
					enableKubeGatewayApi:         &wrappers.BoolValue{Value: true},
					enablePolicyApi:              nil, // default is true
					expectedWebhookConfiguration: containPolicyApiOperation(),
				}),
				Entry("enableKubeGatewayApi=true, enablePolicyApi=true", enablePolicyApiCase{
					enableKubeGatewayApi:         &wrappers.BoolValue{Value: true},
					enablePolicyApi:              &wrappers.BoolValue{Value: true},
					expectedWebhookConfiguration: containPolicyApiOperation(),
				}),
				// This is the critical test case, which demonstrates that a user can enabled the K8s Gateway Integration,
				// but disable validation for the Policy APIs
				Entry("enableKubeGatewayApi=false, enablePolicyApi=true", enablePolicyApiCase{
					enableKubeGatewayApi:         &wrappers.BoolValue{Value: false},
					enablePolicyApi:              &wrappers.BoolValue{Value: true},
					expectedWebhookConfiguration: Not(containPolicyApiOperation()),
				}),
				Entry("enableKubeGatewayApi=false, enablePolicyApi=true", enablePolicyApiCase{
					enableKubeGatewayApi:         &wrappers.BoolValue{Value: false},
					enablePolicyApi:              &wrappers.BoolValue{Value: true},
					expectedWebhookConfiguration: Not(containPolicyApiOperation()),
				}),
				Entry("enableKubeGatewayApi=false, enablePolicyApi=false", enablePolicyApiCase{
					enableKubeGatewayApi:         &wrappers.BoolValue{Value: false},
					enablePolicyApi:              &wrappers.BoolValue{Value: false},
					expectedWebhookConfiguration: Not(containPolicyApiOperation()),
				}),
			)
		})

	}
	runTests(allTests)
})

type expectedChartArgs struct {
	timeoutSeconds    int
	skipDeletes       []string
	expectedDeletes   int
	glooFailurePolicy string
	kubeFailurePolicy string
}

func generateExpectedChart(args expectedChartArgs) *unstructured.Unstructured {

	// Default failure policies
	if args.glooFailurePolicy == "" {
		args.glooFailurePolicy = "Ignore"
	}
	if args.kubeFailurePolicy == "" {
		args.kubeFailurePolicy = "Ignore"
	}

	GinkgoHelper()
	glooRules, coreRules := generateRules(args.skipDeletes)

	// indent "rules"
	m1 := regexp.MustCompile("\n")
	glooRules = m1.ReplaceAllString(glooRules, "\n    ")
	coreRules = m1.ReplaceAllString(coreRules, "\n    ")

	// Check that we have the expected number of DELETEs
	m2 := regexp.MustCompile(`DELETE`)
	deletes := len(m2.FindAllStringIndex(glooRules, -1)) + len(m2.FindAllStringIndex(coreRules, -1))
	Expect(deletes).To(Equal(args.expectedDeletes))

	chart := `
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  name: gloo-gateway-validation-webhook-` + namespace + `
  labels:
    app: gloo
    gloo: gateway
  annotations:
    "helm.sh/hook": pre-install, pre-upgrade
    "helm.sh/hook-weight": "5" # should come before cert-gen job
webhooks:
- name: gloo.` + namespace + `.svc  # must be a domain with at least three segments separated by dots
  clientConfig:
    service:
      name: gloo
      namespace: ` + namespace + `
      path: "/validation"
    caBundle: "" # update manually or use certgen job
  rules:
    ` + glooRules + `
  sideEffects: None
  matchPolicy: Exact
  timeoutSeconds: ` + strconv.Itoa(args.timeoutSeconds) + `
  admissionReviewVersions:
    - v1beta1
  failurePolicy: ` + args.glooFailurePolicy + `
`
	// Only create the webhook for core resources if there are any resources being valdiated
	if coreRules != "[]\n    " {
		chart += `- name: kube.` + namespace + `.svc  # must be a domain with at least three segments separated by dots
  clientConfig:
    service:
      name: gloo
      namespace: ` + namespace + `
      path: "/validation"
    caBundle: "" # update manually or use certgen job
  rules:
    ` + coreRules + `
  sideEffects: None
  matchPolicy: Exact
  timeoutSeconds: ` + strconv.Itoa(args.timeoutSeconds) + `
  admissionReviewVersions:
    - v1beta1
  failurePolicy: ` + args.kubeFailurePolicy + `
`
	}

	return makeUnstructured(chart)
}

// generateRules returns gloo rules and core rules as separate strings
func generateRules(skipDeleteReources []string) (string, string) {
	GinkgoHelper()
	glooRules := []map[string][]string{
		{
			"operations":  {"CREATE", "UPDATE", "DELETE"},
			"apiGroups":   {"gateway.solo.io"},
			"apiVersions": {"v1"},
			"resources":   {"virtualservices"},
		},
		{
			"operations":  {"CREATE", "UPDATE", "DELETE"},
			"apiGroups":   {"gateway.solo.io"},
			"apiVersions": {"v1"},
			"resources":   {"routetables"},
		},
		{
			"operations":  {"CREATE", "UPDATE"},
			"apiGroups":   {"gateway.solo.io"},
			"apiVersions": {"v1"},
			"resources":   {"gateways"},
		},
		{
			"operations":  {"CREATE", "UPDATE", "DELETE"},
			"apiGroups":   {"gloo.solo.io"},
			"apiVersions": {"v1"},
			"resources":   {"upstreams"},
		},
		{
			"operations":  {"CREATE", "UPDATE", "DELETE"},
			"apiGroups":   {"ratelimit.solo.io"},
			"apiVersions": {"v1alpha1"},
			"resources":   {"ratelimitconfigs"},
		},
	}

	coreRules := []map[string][]string{
		{
			"operations":  {"DELETE"},
			"apiGroups":   {""},
			"apiVersions": {"v1"},
			"resources":   {"secrets"},
		},
		{
			"operations":  {"UPDATE", "DELETE"},
			"apiGroups":   {""},
			"apiVersions": {"v1"},
			"resources":   {"namespaces"},
		},
	}

	finalGlooRules := []map[string][]string{}
	for i, rule := range glooRules {
		if stringutils.ContainsAny([]string{rule["resources"][0], "*"}, skipDeleteReources) {
			rule["operations"] = gloostringutils.DeleteOneByValue(rule["operations"], "DELETE")
		}

		if len(rule["operations"]) != 0 {
			finalGlooRules = append(finalGlooRules, glooRules[i])
		}
	}

	finalNonGlooRules := []map[string][]string{}
	for i, rule := range coreRules {
		if stringutils.ContainsAny([]string{rule["resources"][0], "*"}, skipDeleteReources) {
			rule["operations"] = gloostringutils.DeleteOneByValue(rule["operations"], "DELETE")
			// A namespace with an update to a label can cause it to no longer be watched,
			// equivalent to deleting it from the controller's perspective
			if rule["resources"][0] == "namespaces" {
				rule["operations"] = gloostringutils.DeleteOneByValue(rule["operations"], "UPDATE")
			}
		}

		if len(rule["operations"]) != 0 {
			finalNonGlooRules = append(finalNonGlooRules, coreRules[i])
		}
	}

	glooRulesYaml, err := yaml.Marshal(finalGlooRules)
	Expect(err).NotTo(HaveOccurred())
	nonGlooRulesYaml, err := yaml.Marshal(finalNonGlooRules)
	Expect(err).NotTo(HaveOccurred())
	return string(glooRulesYaml), string(nonGlooRulesYaml)
}
