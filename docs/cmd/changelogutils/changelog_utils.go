package changelogutils

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/google/go-github/v31/github"
	"github.com/rotisserie/eris"
	. "github.com/solo-io/go-utils/versionutils"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// Fetches releases for repo from github
func GetAllReleases(client *github.Client, repo string) ([]*github.RepositoryRelease, error) {
	allReleases, _, err := client.Repositories.ListReleases(context.Background(), "solo-io", repo,
		&github.ListOptions{
			Page:    0,
			PerPage: 10000000,
		})
	if err != nil {
		return nil, err
	}

	return allReleases, nil
}

// Parses releases into correct format for printing
// If byMinorVersion is true, the version header (e.g. v1.5.9-beta8) is not included in the release notes body
func ParseReleases(releases []*github.RepositoryRelease, byMinorVersion bool) (map[Version]string, error) {
	var minorReleaseMap = make(map[Version]string)
	for _, release := range releases {
		var releaseTag = release.GetTagName()
		version, err := ParseVersion(releaseTag)
		if err != nil {
			return nil, err
		}
		var header string
		// If byMinorVersion, we only want to include the release notes in the string and not the release header
		if byMinorVersion {
			header = fmt.Sprintf("##### %v\n", GetReleaseMdLink(version.String(), "gloo"))
			version.LabelVersion, version.Patch, version.Label = 0, 0, ""
		}
		minorReleaseMap[*version] = minorReleaseMap[*version] + header + release.GetBody()
	}

	return minorReleaseMap, nil
}

// Performs processing to generate a map of release version to the release notes
// This also pulls in open source gloo edge release notes and merges them with enterprise release notes
// The returned map will be a mapping of minor releases (v1.5, v1.6) to their body, which will contain the release notes
// for all the patches under the minor releases. It also returns a list of versions, sorted by version if the sortedByVersion param is true.
func MergeEnterpriseOSSReleases(enterpriseReleases, osReleasesSorted []*github.RepositoryRelease, sortedByVersion bool) (map[Version]string, []Version, error) {
	var minorReleaseMap = make(map[Version]string)
	var versionOrder []Version

	enterpriseReleasesSorted := SortReleases(enterpriseReleases)
	// if we don't want to sort it by version, preserve original chronological ordering
	if !sortedByVersion {
		for _, release := range enterpriseReleases {
			version, err := ParseVersion(release.GetTagName())
			if err != nil {
				return nil, nil, err
			}
			versionOrder = append(versionOrder, *version)
		}
	}
	openSourceReleases, err := ParseReleases(osReleasesSorted, false)
	if err != nil {
		return nil, nil, err
	}

	for index, release := range enterpriseReleasesSorted {
		var releaseTag = release.GetTagName()

		version, err := ParseVersion(releaseTag)
		if err != nil {
			return nil, nil, err
		}

		previousEnterprisePatch := GetPreviousEnterprisePatchVersion(enterpriseReleasesSorted, index)

		// Get the Gloo OSS version that the Gloo enterprise version relies on
		depVersion, err := GetOSSDependencyForEnterpriseVersion(version)
		var OSSDescription string
		if err == nil && previousEnterprisePatch != nil {
			OSSDescription = GetOSDependencyPrefix(*depVersion, true)
		}

		// TODO: Implement a cache for gloo enterprise dependencies to reduce the amount of API calls
		// Swallowing error here because we handle the case where previousDepVersion is nil
		previousDepVersion, _ := GetOSSDependencyForEnterpriseVersion(previousEnterprisePatch)
		var depVersions []Version
		// Get all intermediate versions of Gloo OSS that this Gloo enterprise relies on
		if depVersion != nil && previousDepVersion != nil {
			depVersions = GetAllOSSDependenciesBetweenEnterpriseVersions(depVersion, previousDepVersion, osReleasesSorted)
		}
		// Get release notes of the dependent open source gloo release version
		body := AccumulateNotes(release.GetBody(), openSourceReleases, depVersions)
		if sortedByVersion {
			// If sorting by minor version, we only want the minor version (not patch number or label) for the resulting map
			minorVersion := Version{
				Major: version.Major,
				Minor: version.Minor,
			}
			minorReleaseMap[minorVersion] = minorReleaseMap[minorVersion] + fmt.Sprintf("\n##### %s %s\n ", version.String(), OSSDescription) + body
		} else {
			minorReleaseMap[*version] = fmt.Sprintf("\n\n### %s %s\n", version.String(), OSSDescription) + body
		}
	}
	return minorReleaseMap, versionOrder, nil
}

// Parses the enterprise release notes, then inserts open source release notes for each of the dependent versions
// of gloo Open source between the current release and the previous release
func AccumulateNotes(enterpriseReleaseNotes string, openSourceReleaseMap map[Version]string, depVersions []Version) string {
	headersToNotesMap := make(map[string][]string)
	extraText := ParseReleaseNotes(enterpriseReleaseNotes, headersToNotesMap, "\n- ")
	for _, osVersionDependency := range depVersions {
		prefix := GetOSDependencyPrefix(osVersionDependency, false)
		ParseReleaseNotes(openSourceReleaseMap[osVersionDependency], headersToNotesMap, prefix)
	}
	headersOrder := []string{"New Features", "Fixes", "Dependency Bumps", "Helm Changes"}

	var finalReleaseNotes []string
	for _, header := range headersOrder {
		if notes := headersToNotesMap[header]; notes != nil {
			headerString := fmt.Sprintf("\n**%s**", header)
			releaseNotesForCurrentHeader := strings.Join(notes, "\n")
			finalReleaseNotes = append(finalReleaseNotes, headerString, releaseNotesForCurrentHeader)
			delete(headersToNotesMap, header)
		}
	}
	for header, notes := range headersToNotesMap {
		headerString := fmt.Sprintf("\n**%s**", header)
		releaseNotesForCurrentHeader := strings.Join(notes, "\n")
		finalReleaseNotes = append(finalReleaseNotes, headerString, releaseNotesForCurrentHeader)
	}

	return extraText + strings.Join(finalReleaseNotes, "\n")
}

/*
Parses the release notes for a release version into a map `headersToNotesMap` which
maps each of the headers (e.g. Fixes, Dependency Bumps, New Features) to the release notes
under the header
*/
func ParseReleaseNotes(releaseNotes string, headersToNotesMap map[string][]string, prefix string) string {
	releaseNotesBuf := []byte(releaseNotes)
	rootNode := goldmark.DefaultParser().Parse(text.NewReader(releaseNotesBuf))
	var currentHeader string // e.g. New Features, Fixes, Helm Changes, Dependency Bumps, CVEs
	var accumulator string   // accumulator for any extra text e.g. "This release build has failed", only used for enterprise release notes
	for n := rootNode.FirstChild(); n != nil; n = n.NextSibling() {
		switch typedNode := n.(type) {
		case *ast.Paragraph:
			{
				if child := typedNode.FirstChild(); child.Kind() == ast.KindEmphasis {
					emphasis := child.(*ast.Emphasis)
					if emphasis.Level == 2 {
						// Header block
						currentHeader = string(typedNode.Text(releaseNotesBuf))
						continue
					}

				}
				// This section will handles any paragraphs that do not show up under headers e.g. "This release build failed"
				v := typedNode.Lines().At(0)
				note := prefix + fmt.Sprintf("%s\n", v.Value(releaseNotesBuf))
				if currentHeader != "" {
					headersToNotesMap[currentHeader] = append(headersToNotesMap[currentHeader], note)
				} else {
					//any extra text e.g. "This release build has failed", only used for enterprise release notes
					accumulator = accumulator + note
				}
			}
		case *ast.List:
			{
				// Only add release notes if we are under a current header
				if currentHeader != "" {
					for child := n.FirstChild(); child != nil; child = child.NextSibling() {
						v := child.FirstChild().Lines().At(0)
						releaseNoteNode := v.Value(releaseNotesBuf)
						releaseNote := prefix + string(releaseNoteNode)
						headersToNotesMap[currentHeader] = append(headersToNotesMap[currentHeader], releaseNote)
					}
				}
			}
		}
	}
	return accumulator
}

func GetOSDependencyPrefix(openSourceVersion Version, isHeader bool) string {
	prefix := "\n- (From"
	if isHeader {
		prefix = " (Uses Gloo Edge"
	}
	osReleaseURL := strings.ReplaceAll(openSourceVersion.String(), ".", "")
	osPrefix := fmt.Sprintf("%s [OSS %s](../../../reference/changelog/open_source/#%s)) ", prefix, openSourceVersion.String(), osReleaseURL)
	return osPrefix
}

func GetPreviousEnterprisePatchVersion(enterpriseReleasesSorted []*github.RepositoryRelease, index int) *Version {
	var previousEnterpriseVersion *Version
	currentVersion, err := ParseVersion(enterpriseReleasesSorted[index].GetTagName())
	if err != nil {
		return nil
	}
	if index+1 != len(enterpriseReleasesSorted) {
		previousRelease := enterpriseReleasesSorted[index+1]
		previousVersion, err := ParseVersion(previousRelease.GetTagName())
		if err != nil {
			return nil
		}
		// The previous enterprise version only concerns us if it was a patch of the same major and minor version
		if previousVersion.Major != currentVersion.Major || previousVersion.Minor != currentVersion.Minor {
			return nil
		}
		previousEnterpriseVersion = previousVersion
	}
	return previousEnterpriseVersion
}

// Get the list of open source versions between open source version that the previous enterprise version used and the current enterprise version uses
func GetAllOSSDependenciesBetweenEnterpriseVersions(startVersion, endVersion *Version, versionsSorted []*github.RepositoryRelease) []Version {
	var dependencies []Version

	var adding bool
	for _, release := range versionsSorted {
		tag, err := ParseVersion(release.GetTagName())
		if err != nil {
			continue
		}
		version := *tag
		if version == *startVersion {
			adding = true
		}
		if adding && (version.Major != startVersion.Major || version.Minor != startVersion.Minor) {
			break
		}
		if version == *endVersion {
			break
		}
		if adding {
			dependencies = append(dependencies, *tag)
		}
	}
	return dependencies
}

func GetOSSDependencyForEnterpriseVersion(enterpriseVersion *Version) (*Version, error) {
	if enterpriseVersion == nil {
		return nil, nil
	}
	versionTag := enterpriseVersion.String()
	dependencyUrl := fmt.Sprintf("https://storage.googleapis.com/gloo-ee-dependencies/%s/dependencies", versionTag[1:])
	request, err := http.NewRequest("GET", dependencyUrl, nil)
	if err != nil {
		return nil, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	re, err := regexp.Compile(`.*gloo.*(v.*)`)
	if err != nil {
		return nil, err
	}
	matches := re.FindStringSubmatch(string(body))
	if len(matches) != 2 {
		return nil, eris.Errorf("unable to get gloo dependency for gloo enterprise version %s\n response from google storage API: %s", versionTag, string(body))
	}
	glooVersionTag := matches[1]
	version, err := ParseVersion(glooVersionTag)
	if err != nil {
		return nil, err
	}
	return version, nil
}

// Sorts a slice of versions in descending order by version e.g. v1.6.1, v1.6.0, v1.6.0-beta9
func SortReleaseVersions(versions []Version) []Version {
	versionsCopy := make([]Version, len(versions))
	copy(versionsCopy, versions)
	sort.Slice(versionsCopy, func(i, j int) bool {
		return versionsCopy[i].MustIsGreaterThanOrEqualTo(versionsCopy[j])
	})
	return versionsCopy
}

func SortReleases(releases []*github.RepositoryRelease) []*github.RepositoryRelease {
	releasesCopy := make([]*github.RepositoryRelease, len(releases))
	copy(releasesCopy, releases)
	sort.Slice(releasesCopy, func(i, j int) bool {
		releaseA, releaseB := releasesCopy[i], releasesCopy[j]
		versionA, err := ParseVersion(releaseA.GetTagName())
		if err != nil {
			return false
		}
		versionB, err := ParseVersion(releaseB.GetTagName())
		if err != nil {
			return false
		}
		return versionA.MustIsGreaterThan(*versionB)
	})
	return releasesCopy
}

func GetReleaseMdLink(releaseTag, repo string) string {
	return fmt.Sprintf("[%s](https://github.com/solo-io/%s/releases/tag/%s)", releaseTag, repo, releaseTag)
}
