package upgrade

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"strconv"

	"net/http"
	"net/http/httptest"

	"github.com/google/go-github/github"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
)

var _ = Describe("Cmd", func() {

	var (
		ctx    context.Context
		cancel context.CancelFunc
	)

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
	ts := httptest.NewUnstartedServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// respond with a sub section of the list based on the page query
			page := r.URL.Query().Get("page")
			pageInt, _ := strconv.Atoi(page)

			startingIdx := pageInt * 10
			if startingIdx > len(releases) {
				startingIdx = len(releases)
			}
			endingIdx := startingIdx + 10
			if endingIdx > len(releases) {
				endingIdx = len(releases)
			}
			releaseJson, _ := json.Marshal(releases[startingIdx:endingIdx])
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

	table.DescribeTable("release types",
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
		table.Entry("experimental gets largest semver", "experimental", "v1.11.0-beta11", ""),
		table.Entry("latest gets latest stable", "latest", "v1.10.18", ""),
		table.Entry("v1.10.x gets latest stable", "v1.10.x", "v1.10.18", ""),
		table.Entry("v1.9.x gets in minor", "v1.9.x", "v1.9.7", ""),
		table.Entry("v1.2.x is not found", "v1.2.x", "", errorNotFoundString),
		table.Entry("v2.2.x is not found", "v2.2.x", "", errorNotFoundString),
	)
})
