package cli_unit_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/install"
)

var _ = Describe("Manifest", func() {

	const (
		initial = `    spec:
      containers:
      - image: soloio/gloo:0.5.5-2-g60e818f7
        imagePullPolicy: Always
        name: gloo
        ports:
    spec:
      containers:
      - image: soloio/foo:0.5.5-2-g60e818f7
        imagePullPolicy: Always
        name: gloo
        ports:`

		updated = `    spec:
      containers:
      - image: soloio/gloo:0.5.6
        imagePullPolicy: Always
        name: gloo
        ports:
    spec:
      containers:
      - image: soloio/foo:0.5.6
        imagePullPolicy: Always
        name: gloo
        ports:`
	)

	It("undefined version doesn't replace", func() {
		actual := install.UpdateBytesWithVersion([]byte(initial), "undefined")
		Expect(actual).To(BeEquivalentTo(initial))
	})

	It("defined version does replace", func() {
		actual := install.UpdateBytesWithVersion([]byte(initial), "0.5.6")
		Expect(actual).To(BeEquivalentTo(updated))
	})
})
