package upgrade

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-github/github"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/solo-io/gloo/test/gomega"
)

func TestCheck(t *testing.T) {
	SetAsyncAssertionDefaults(AsyncAssertionDefaults{})
	RegisterFailHandler(Fail)

	RunSpecs(t, "Upgrade Suite")
}

var (
	ts *httptest.Server

	name1125      = "v1.12.5"
	name200Beta1  = "v2.0.0-beta1"
	name1108      = "v1.10.18"
	name1107      = "v1.10.7"
	name197       = "v1.9.7"
	name160filler = "v1.6.0"
	name111Beta11 = "v1.11.0-beta11"

	glooctlBinaryName = fmt.Sprintf("glooctl-%v-%v", runtime.GOOS, runtime.GOARCH)

	releases = []github.RepositoryRelease{
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
	releasesPostMajorBump = []github.RepositoryRelease{
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
)

var _ = BeforeSuite(func() {
	ts = httptest.NewUnstartedServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			releaseSet := releases
			if strings.Contains(r.URL.Path, "newMajor") {
				releaseSet = releasesPostMajorBump
			}

			// respond with a subsection of the list based on the page query
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
})

var _ = AfterSuite(func() {
	ts.Close()
})
