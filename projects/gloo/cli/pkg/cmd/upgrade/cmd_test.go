package upgrade

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"strconv"
	"strings"

	"net/http"
	"net/http/httptest"

	"github.com/google/go-github/github"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Cmd", func() {

	var (
		ctx    context.Context
		cancel context.CancelFunc
	)
	name1125 := "v1.12.5"
	name200Beta1 := "v2.0.0-beta1"
	name1108 := "v1.10.18"
	name1107 := "v1.10.7"
	name197 := "v1.9.7"
	name160filler := "v1.6.0"
	name111Beta11 := "v1.11.0-beta11"

	glooctlBinaryName := fmt.Sprintf("glooctl-%v-%v", runtime.GOOS, runtime.GOARCH)
	releases := []github.RepositoryRelease{
		{Name: &name160filler, Assets: []github.ReleaseAsset{{Name: &glooctlBinaryName}}},
		{Name: &name160filler, Assets: []github.ReleaseAsset{{Name: &glooctlBinaryName}}},
		{Name: &name160filler, Assets: []github.ReleaseAsset{{Name: &glooctlBinaryName}}},
		{Name: &name160filler, Assets: []github.ReleaseAsset{{Name: &glooctlBinaryName}}},
		{Name: &name160filler, Assets: []github.ReleaseAsset{{Name: &glooctlBinaryName}}},
		{Name: &name160filler, Assets: []github.ReleaseAsset{{Name: &glooctlBinaryName}}},
		{Name: &name160filler, Assets: []github.ReleaseAsset{{Name: &glooctlBinaryName}}},
		{Name: &name160filler, Assets: []github.ReleaseAsset{{Name: &glooctlBinaryName}}},
		{Name: &name160filler, Assets: []github.ReleaseAsset{{Name: &glooctlBinaryName}}},
		{Name: &name160filler, Assets: []github.ReleaseAsset{{Name: &glooctlBinaryName}}},
		// page 2 as its 11 in
		{Name: &name160filler, Assets: []github.ReleaseAsset{{Name: &glooctlBinaryName}}},
		{Name: &name197, Assets: []github.ReleaseAsset{{Name: &glooctlBinaryName}}},
		{Name: &name160filler, Assets: []github.ReleaseAsset{{Name: &glooctlBinaryName}}},
		{Name: &name160filler, Assets: []github.ReleaseAsset{{Name: &glooctlBinaryName}}},
		{Name: &name160filler, Assets: []github.ReleaseAsset{{Name: &glooctlBinaryName}}},
		{Name: &name160filler, Assets: []github.ReleaseAsset{{Name: &glooctlBinaryName}}},
		{Name: &name160filler, Assets: []github.ReleaseAsset{{Name: &glooctlBinaryName}}},
		{Name: &name160filler, Assets: []github.ReleaseAsset{{Name: &glooctlBinaryName}}},
		{Name: &name111Beta11, Assets: []github.ReleaseAsset{{Name: &glooctlBinaryName}}},
		{Name: &name1108, Assets: []github.ReleaseAsset{{Name: &glooctlBinaryName}}},
		{Name: &name1107, Assets: []github.ReleaseAsset{{Name: &glooctlBinaryName}}},
	}
	releasesPostMajorBump := []github.RepositoryRelease{
		{Name: &name160filler, Assets: []github.ReleaseAsset{{Name: &glooctlBinaryName}}},
		{Name: &name160filler, Assets: []github.ReleaseAsset{{Name: &glooctlBinaryName}}},
		{Name: &name160filler, Assets: []github.ReleaseAsset{{Name: &glooctlBinaryName}}},
		{Name: &name160filler, Assets: []github.ReleaseAsset{{Name: &glooctlBinaryName}}},
		{Name: &name160filler, Assets: []github.ReleaseAsset{{Name: &glooctlBinaryName}}},
		{Name: &name160filler, Assets: []github.ReleaseAsset{{Name: &glooctlBinaryName}}},
		{Name: &name160filler, Assets: []github.ReleaseAsset{{Name: &glooctlBinaryName}}},
		{Name: &name160filler, Assets: []github.ReleaseAsset{{Name: &glooctlBinaryName}}},
		{Name: &name160filler, Assets: []github.ReleaseAsset{{Name: &glooctlBinaryName}}},
		{Name: &name160filler, Assets: []github.ReleaseAsset{{Name: &glooctlBinaryName}}},
		// page 2 as its 11 in
		{Name: &name160filler, Assets: []github.ReleaseAsset{{Name: &glooctlBinaryName}}},
		{Name: &name197, Assets: []github.ReleaseAsset{{Name: &glooctlBinaryName}}},
		{Name: &name160filler, Assets: []github.ReleaseAsset{{Name: &glooctlBinaryName}}},
		{Name: &name160filler, Assets: []github.ReleaseAsset{{Name: &glooctlBinaryName}}},
		{Name: &name160filler, Assets: []github.ReleaseAsset{{Name: &glooctlBinaryName}}},
		{Name: &name160filler, Assets: []github.ReleaseAsset{{Name: &glooctlBinaryName}}},
		{Name: &name1125, Assets: []github.ReleaseAsset{{Name: &glooctlBinaryName}}},
		{Name: &name200Beta1, Assets: []github.ReleaseAsset{{Name: &glooctlBinaryName}}},
		{Name: &name1108, Assets: []github.ReleaseAsset{{Name: &glooctlBinaryName}}},
		{Name: &name1108, Assets: []github.ReleaseAsset{{Name: &glooctlBinaryName}}},
		{Name: &name1107, Assets: []github.ReleaseAsset{{Name: &glooctlBinaryName}}},
	}
	ts := httptest.NewUnstartedServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			releaseSet := releases
			if strings.Contains(r.URL.Path, "newMajor") {
				releaseSet = releasesPostMajorBump
			}

			// respond with a sub section of the list based on the page query
			page := r.URL.Query().Get("page")
			pageInt, _ := strconv.Atoi(page)

			startingIdx := pageInt * 10
			if startingIdx > len(releaseSet) {
				startingIdx = len(releaseSet)
			}
			endingIdx := startingIdx + 10
			if endingIdx > len(releaseSet) {
				endingIdx = len(releaseSet)
			}
			releaseJson, _ := json.Marshal(releaseSet[startingIdx:endingIdx])
			fmt.Fprintln(w, string(releaseJson))
		}))

	ts.Start()
	BeforeEach(func() {
		ctx = context.WithValue(context.Background(), "githubURL", ts.URL+"/")
		ctx, cancel = context.WithCancel(ctx)

	})

	AfterEach(func() {
		cancel()
	})
	AfterSuite(func() { ts.Close() })

	DescribeTable("release types",
		func(releaseTag, expectedRelease, expectedErrString string) {
			rel, err := getReleaseWithAsset(ctx, ts.Client(), releaseTag, glooctlBinaryName)
			if err != nil {
				Expect(err.Error()).To(ContainSubstring(expectedErrString))
				Expect(expectedErrString).ShouldNot(BeEmpty())
				Expect(expectedRelease).To(BeEmpty())
			} else {
				Expect(*rel.Name).To(Equal(expectedRelease))
			}

		},
		Entry("experimental gets largest semver", "experimental", "v1.11.0-beta11", ""),
		Entry("latest gets latest stable", "latest", "v1.10.18", ""),
		Entry("v1.10.x gets latest stable", "v1.10.x", "v1.10.18", ""),
		Entry("v1.9.x gets in minor", "v1.9.x", "v1.9.7", ""),
		Entry("v1.2.x is not found", "v1.2.x", "", errorNotFoundString),
		Entry("v2.2.x is not found", "v2.2.x", "", errorNotFoundString),
	)

	It("Can handle new major versions", func() {
		ctx = context.WithValue(ctx, "githubURL", ts.URL+"/newMajor/")

		relExp, _ := getReleaseWithAsset(ctx, ts.Client(), "experimental", glooctlBinaryName)
		Expect(*relExp.Name).To(Equal("v2.0.0-beta1"))
		relLatest, _ := getReleaseWithAsset(ctx, ts.Client(), "latest", glooctlBinaryName)
		Expect(*relLatest.Name).To(Equal("v1.12.5"))
	})
})
