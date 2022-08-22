package make_test

import (
	"fmt"
	"os/exec"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// Environment Variables which control the value of makefile vars
const (
	TaggedVersion     = "TAGGED_VERSION"
	TestAssetId       = "TEST_ASSET_ID"
	UpstreamOriginUrl = "UPSTREAM_ORIGIN_URL"
	OriginUrl         = "ORIGIN_URL"
)

// Makefile vars
const (
	CreateAssets     = "CREATE_ASSETS"
	CreateTestAssets = "CREATE_TEST_ASSETS"
	Release          = "RELEASE"
	HelmBucket       = "HELM_BUCKET"
	Version          = "VERSION"
)

// NOTE: These tests are mostly to check that the makefile ifeq directive logic is working as expected.
// It's preferred that if Makefile logic gets anymore complicated, to direct the logic to go code rather than
// increase the complexity of the makefile.
var _ = Describe("Make", func() {

	It("should set RELEASE to true iff TAGGED_VERSION is set", func() {
		ExpectMakeVarsWithEnvVars([]*EnvVar{
			{TaggedVersion, "v0.0.1-someVersion"},
		}, []*MakeVar{
			{Release, "true"},
		})

		ExpectMakeVarsWithEnvVars(nil, []*MakeVar{
			{Release, "false"},
		})
	})

	It("should set CREATE_TEST_ASSETS to true iff TEST_ASSET_ID is set", func() {
		// if we are maintainers and set test asset id, create test assets
		ExpectMakeVarsWithEnvVars([]*EnvVar{
			{OriginUrl, "git@github.com:solo-io/gloo.git"}, // required so ci passes from a fork
			{UpstreamOriginUrl, "git@github.com:solo-io/gloo.git"},
			{TestAssetId, "4300"},
		}, []*MakeVar{
			{CreateTestAssets, "true"},
		})

		// no need to create test assets from fork
		ExpectMakeVarsWithEnvVars([]*EnvVar{
			{TestAssetId, "4300"},
			{OriginUrl, "git@github.com:kdorosh/gloo.git"},
			{UpstreamOriginUrl, "git@github.com:solo-io/gloo.git"},
		}, []*MakeVar{
			{CreateTestAssets, "false"},
		})

		// don't create test assets by default
		ExpectMakeVarsWithEnvVars(nil, []*MakeVar{
			{CreateTestAssets, "false"},
		})
	})

	It("should create assets if TAGGED_VERSION || TEST_ASSET_ID", func() {
		ExpectMakeVarsWithEnvVars([]*EnvVar{
			{TaggedVersion, "v0.0.1-someVersion"},
			{OriginUrl, "git@github.com:solo-io/gloo.git"}, // required so ci passes from a fork
			{UpstreamOriginUrl, "git@github.com:solo-io/gloo.git"},
		}, []*MakeVar{
			{CreateAssets, "true"},
		})

		ExpectMakeVarsWithEnvVars([]*EnvVar{
			{TestAssetId, "4300"},
			{OriginUrl, "git@github.com:solo-io/gloo.git"}, // required so ci passes from a fork
			{UpstreamOriginUrl, "git@github.com:solo-io/gloo.git"},
		}, []*MakeVar{
			{CreateAssets, "true"},
		})

		ExpectMakeVarsWithEnvVars([]*EnvVar{
			{OriginUrl, "git@github.com:solo-io/gloo.git"}, // required so ci passes from a fork
			{UpstreamOriginUrl, "git@github.com:solo-io/gloo.git"},
		}, []*MakeVar{
			{CreateAssets, "false"},
		})
	})

	Context("VERSION", func() {
		It("should be set according to TAGGED_VERSION", func() {

			out, err := exec.Command("git", "remote", "get-url", "origin").CombinedOutput()
			Expect(err).NotTo(HaveOccurred())

			remoteUrl := string(out)
			if !strings.Contains(remoteUrl, "git@github.com:solo-io/gloo.git") {
				// we are on a fork
				Skip("skip")
			}

			ExpectMakeVarsWithEnvVars([]*EnvVar{
				{TaggedVersion, "v0.0.1-someVersion"},
			}, []*MakeVar{
				{Version, "0.0.1-someVersion"},
			})
		})

		It("should be set according to TEST_ASSET_ID", func() {

			out, err := exec.Command("git", "remote", "get-url", "origin").CombinedOutput()
			Expect(err).NotTo(HaveOccurred())

			expectedVersion := "0.0.0-fork"
			remoteUrl := string(out)
			if strings.Contains(remoteUrl, "git@github.com:solo-io/gloo.git") {
				out, err := exec.Command("git", "describe", "--tags", "--abbrev=0").CombinedOutput()
				Expect(err).NotTo(HaveOccurred())
				gitDesc := strings.TrimSpace(string(out))
				gitDesc = strings.TrimPrefix(gitDesc, "v")
				expectedVersion = fmt.Sprintf("%s-%d", gitDesc, 4300)
			}

			ExpectMakeVarsWithEnvVars([]*EnvVar{
				{TestAssetId, "4300"},
			}, []*MakeVar{
				{Version, expectedVersion},
			})
		})

		It("neither TAGGED_VERSION nor TEST_ASSET_ID are set", func() {

			out, err := exec.Command("git", "remote", "get-url", "origin").CombinedOutput()
			Expect(err).NotTo(HaveOccurred())

			remoteUrl := string(out)
			if !strings.Contains(remoteUrl, "git@github.com:solo-io/gloo.git") {
				// we are on a fork
				Skip("skip")
			}

			out, err = exec.Command("git", "describe", "--tags", "--dirty").CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
			gitDesc := strings.TrimSpace(string(out))
			gitDesc = strings.TrimPrefix(gitDesc, "v")
			ExpectMakeVarsWithEnvVars([]*EnvVar{}, []*MakeVar{
				{Version, gitDesc},
			})
		})

		It("should be overridden by pre-existing VERSION environment variable", func() {
			ExpectMakeVarsWithEnvVars([]*EnvVar{
				{OriginUrl, "git@github.com:solo-io/gloo.git"}, // required so ci passes from a fork
				{UpstreamOriginUrl, "git@github.com:solo-io/gloo.git"},
				{Version, "kind"},
				{TestAssetId, "4300"},
				{TaggedVersion, "v0.0.1-someVersion"},
			}, []*MakeVar{
				{Version, "kind"},
			})
		})
	})

	Context("HELM_BUCKET", func() {
		It("is official helm chart repo on RELEASE", func() {
			ExpectMakeVarsWithEnvVars([]*EnvVar{
				{TaggedVersion, "v0.0.1-someVersion"},
			}, []*MakeVar{
				{HelmBucket, "gs://solo-public-helm"},
			})
		})

		It("is temp helm chart repo on TEST_ASSET_ID", func() {
			ExpectMakeVarsWithEnvVars([]*EnvVar{
				{TestAssetId, "4300"},
			}, []*MakeVar{
				{HelmBucket, "gs://solo-public-tagged-helm"},
			})
		})
	})

})
