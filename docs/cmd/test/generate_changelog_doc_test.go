package main_test

import (
	"github.com/google/go-github/v31/github"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/docs/cmd/changelogutils"
	. "github.com/solo-io/go-utils/versionutils"
)

var _ = Describe("Generate Changelog Test", func() {
	Context("Version functions", func() {

		var (
			sortedVersionsArray                                        []Version
			v1_6_5, v1_6_3, v1_5_5, v1_5_0, v1_5_0_beta9, v1_5_0_beta8 Version
			enterpriseReleasesSorted                                   []*github.RepositoryRelease
		)
		BeforeEach(func() {
			v1_6_5 = Version{Major: 1, Minor: 6, Patch: 5}
			v1_6_3, v1_5_5 = v1_6_5, v1_6_5
			v1_6_3.Patch = 3
			v1_5_5.Minor = 5
			v1_5_0 = v1_5_5
			v1_5_0.Patch = 0
			v1_5_0_beta9 = v1_5_0
			v1_5_0_beta9.Label = "beta"
			v1_5_0_beta9.LabelVersion = 9
			v1_5_0_beta8 = v1_5_0_beta9
			v1_5_0_beta8.LabelVersion = 8

			sortedVersionsArray = []Version{
				v1_6_5, v1_6_3, v1_5_5, v1_5_0, v1_5_0_beta9, v1_5_0_beta8,
			}

			for _, version := range sortedVersionsArray {
				tagName := version.String()
				enterpriseReleasesSorted = append(enterpriseReleasesSorted,
					&github.RepositoryRelease{
						TagName: &tagName,
					})
			}
		})

		It("sorts release versions", func() {

			versions := []Version{
				v1_5_0_beta9, v1_5_5, v1_5_0, v1_6_3, v1_6_5, v1_5_0_beta8,
			}
			changelogutils.SortReleaseVersions(versions)
			Expect(versions).To(HaveLen(len(sortedVersionsArray)))
			Expect(versions).To(Equal(sortedVersionsArray))
		})

		It("calculates previous enterprise patch", func() {
			previousVersion := changelogutils.GetPreviousEnterprisePatchVersion(enterpriseReleasesSorted, 0)
			Expect(previousVersion.String()).To(Equal("v1.6.3"))
			previousVersion = changelogutils.GetPreviousEnterprisePatchVersion(enterpriseReleasesSorted, 3)
			Expect(previousVersion.String()).To(Equal("v1.5.0-beta9"))

			By("returns nil for enterprise releases with previous patch within the minor release")

			previousVersion = changelogutils.GetPreviousEnterprisePatchVersion(enterpriseReleasesSorted, 1)
			Expect(previousVersion).To(BeNil())
			previousVersion = changelogutils.GetPreviousEnterprisePatchVersion(enterpriseReleasesSorted, len(sortedVersionsArray)-1)
			Expect(previousVersion).To(BeNil())
		})

		It("gets all versions between the current dependency and previous dependency", func() {
			dependencies := changelogutils.GetAllOSSDependenciesBetweenEnterpriseVersions(&v1_5_5, &v1_5_0_beta8, enterpriseReleasesSorted)
			expectedDependencies := []Version{v1_5_5, v1_5_0, v1_5_0_beta9}
			Expect(dependencies).To(Equal(expectedDependencies))
		})

	})

	Context("String processing functions", func() {
		It("creates prefixes for open source dependency notes and headers", func() {
			osVersion := Version{Major: 1, Minor: 5, Patch: 9, Label: "beta", LabelVersion: 3}
			useHeaderPrefix := false
			bulletPointPrefix := changelogutils.GetOSDependencyPrefix(osVersion, useHeaderPrefix)
			expectedBulletPrefix := "\n- (From [OSS v1.5.9-beta3](/reference/changelog/open_source/#v159-beta3)) "
			Expect(bulletPointPrefix).To(Equal(expectedBulletPrefix))

			useHeaderPrefix = true
			headerPrefix := changelogutils.GetOSDependencyPrefix(osVersion, useHeaderPrefix)
			expectedHeaderPrefix := " (Uses Gloo Edge [OSS v1.5.9-beta3](/reference/changelog/open_source/#v159-beta3)) "
			Expect(headerPrefix).To(Equal(expectedHeaderPrefix))
		})
	})

	Context("Markdown processing functions", func() {
		var releaseNotes = `
*This release build failed*

This release contained no user-facing changes

**CVEs**

Some CVE note 1

Some CVE note 2

**Dependency Bumps**

- Some dep bump 1 with **emphasis level 2**
- Some dep bump 2 with *emphasis level 1*
` + "\n - Some dep bump 3 that contains `code`"
		Context("Parsing per release", func() {

			var expectedHeadersToNotesMap = map[string][]string{
				"CVEs":             {"\n- Some CVE note 1\n", "\n- Some CVE note 2\n"},
				"Dependency Bumps": {"\n- Some dep bump 1 with **emphasis level 2**", "\n- Some dep bump 2 with *emphasis level 1*", "\n- Some dep bump 3 that contains `code`"},
			}
			var expectedAccumulatorText = `
- *This release build failed*

- This release contained no user-facing changes
`
			It("parses text into headers -> release notes map", func() {
				headersToNotesMap := make(map[string][]string)
				accumulatorText := changelogutils.ParseReleaseNotes(releaseNotes, headersToNotesMap, "\n- ")

				Expect(headersToNotesMap).To(Equal(expectedHeadersToNotesMap))
				Expect(accumulatorText).To(Equal(expectedAccumulatorText))
			})
		})
	})

})
