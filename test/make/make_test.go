package make

import (
	"fmt"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"os/exec"
	"strings"
)

// Environment Variables which control the value of makefile vars
const (
	TAGGED_VERSION = "TAGGED_VERSION"
	TEST_ASSET_ID  = "TEST_ASSET_ID"
)

// Makefile vars
const (
	CREATE_ASSETS      = "CREATE_ASSETS"
	CREATE_TEST_ASSETS = "CREATE_TEST_ASSETS"
	RELEASE            = "RELEASE"
	HELM_BUCKET        = "HELM_BUCKET"
	VERSION            = "VERSION"

	REPO_NAME = "gloo"
)

var _ = Describe("Make", func() {

	It("should set RELEASE to true iff TAGGED_VERSION is set", func() {
		ExpectMakeVarsWithEnvVars([]*EnvVar{
			{TAGGED_VERSION, "v0.0.1-someVersion"},
		}, []*MakeVar{
			{RELEASE, "true"},
		})

		ExpectMakeVarsWithEnvVars(nil, []*MakeVar{
			{RELEASE, "false"},
		})
	})

	It("should set CREATE_TEST_ASSETS to true iff TEST_ASSET_ID is set", func() {
		ExpectMakeVarsWithEnvVars([]*EnvVar{
			{TEST_ASSET_ID, "4300"},
		}, []*MakeVar{
			{CREATE_TEST_ASSETS, "true"},
		})

		ExpectMakeVarsWithEnvVars(nil, []*MakeVar{
			{CREATE_TEST_ASSETS, "false"},
		})
	})

	It("should create assets if TAGGED_VERSION || TEST_ASSET_ID", func() {
		ExpectMakeVarsWithEnvVars([]*EnvVar{
			{TAGGED_VERSION, "v0.0.1-someVersion"},
		}, []*MakeVar{
			{CREATE_ASSETS, "true"},
		})

		ExpectMakeVarsWithEnvVars([]*EnvVar{
			{TEST_ASSET_ID, "4300"},
		}, []*MakeVar{
			{CREATE_ASSETS, "true"},
		})

		ExpectMakeVarsWithEnvVars(nil, []*MakeVar{
			{CREATE_ASSETS, "false"},
		})
	})

	Context("VERSION", func() {
		It("should be set according to TAGGED_VERSION", func() {
			ExpectMakeVarsWithEnvVars([]*EnvVar{
				{TAGGED_VERSION, "v0.0.1-someVersion"},
			}, []*MakeVar{
				{VERSION, "0.0.1-someVersion"},
			})
		})

		It("should be set according to TEST_ASSET_ID", func() {

			out, err := exec.Command("git", "describe", "--tags", "--abbrev=0").CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			gitDesc := strings.TrimSpace(string(out))
			gitDesc = strings.TrimPrefix(gitDesc, "v")
			ExpectMakeVarsWithEnvVars([]*EnvVar{
				{TEST_ASSET_ID, "4300"},
			}, []*MakeVar{
				{VERSION, fmt.Sprintf("%s-%d", gitDesc, 4300)},
			})
		})

		When("neither TAGGED_VERSION nor TEST_ASSET_ID are set", func() {

			out, err := exec.Command("git", "describe", "--tags", "--dirty").CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			gitDesc := strings.TrimSpace(string(out))
			gitDesc = strings.TrimPrefix(gitDesc, "v")
			ExpectMakeVarsWithEnvVars([]*EnvVar{}, []*MakeVar{
				{VERSION, gitDesc},
			})
		})

		It("should be overridden by pre-existing VERSION environment variable", func() {
			ExpectMakeVarsWithEnvVars([]*EnvVar{
				{VERSION, "kind"},
				{TEST_ASSET_ID, "4300"},
				{TAGGED_VERSION, "v0.0.1-someVersion"},
			}, []*MakeVar{
				{VERSION, "kind"},
			})
		})
	})

	FContext("HELM_BUCKET", func() {
		It("is official helm chart repo on RELEASE", func() {
			ExpectMakeVarsWithEnvVars([]*EnvVar{
				{TAGGED_VERSION, "v0.0.1-someVersion"},
			}, []*MakeVar{
				{HELM_BUCKET, "gs://solo-public-helm"},
			})
		})

		It("is temp helm chart repo on TEST_ASSET_ID", func() {
			ExpectMakeVarsWithEnvVars([]*EnvVar{
				{TEST_ASSET_ID, "4300"},
			}, []*MakeVar{
				{HELM_BUCKET, "gs://solo-public-tagged-helm"},
			})
		})
	})

})
