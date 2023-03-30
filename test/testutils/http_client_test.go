package testutils_test

import (
	"net/http"
	"reflect"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gloohelpers "github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/testutils"
)

var _ = Describe("HttpClientBuilder", func() {

	It("will fail if the client builder has a new top level field", func() {
		// This test is important as it checks whether the client builder has a new top level field.
		// This should happen very rarely, and should be used as an indication that the `Clone` function
		// most likely needs to change to support this new field

		Expect(reflect.TypeOf(testutils.HttpClientBuilder{}).NumField()).To(
			Equal(4),
			"wrong number of fields found",
		)
	})

	It("clones all fields", func() {
		originalCert, _ := gloohelpers.GetCerts(gloohelpers.Params{
			Hosts: "original.com",
			IsCA:  false,
		})
		clonedCert, _ := gloohelpers.GetCerts(gloohelpers.Params{
			Hosts: "clone.com",
			IsCA:  false,
		})

		originalBuilder := testutils.DefaultClientBuilder().
			WithTimeout(time.Second).
			WithTLSRootCa(originalCert).
			WithTLSServerName("original-server-name")
		clonedBuilder := originalBuilder.Clone().
			WithTimeout(time.Second * 5).
			WithTLSRootCa(clonedCert).
			WithTLSServerName("clone-server-name")

		originalHttpClient := originalBuilder.Build()
		clonedHttpClient := clonedBuilder.Build()

		// Cloning the originalBuilder means we should be modifying only the clone, and not the original originalBuilder
		Expect(originalHttpClient.Timeout).To(BeEquivalentTo(time.Second))
		Expect(clonedHttpClient.Timeout).To(BeEquivalentTo(time.Second * 5))

		originalTlsConfig := originalHttpClient.Transport.(*http.Transport).TLSClientConfig
		clonedTlsConfig := clonedHttpClient.Transport.(*http.Transport).TLSClientConfig
		Expect(originalTlsConfig.RootCAs).NotTo(BeEquivalentTo(clonedTlsConfig.RootCAs))
		Expect(originalTlsConfig.ServerName).NotTo(BeEquivalentTo(clonedTlsConfig.ServerName))
	})

})
