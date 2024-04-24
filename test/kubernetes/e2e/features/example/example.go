package example

import (
	. "github.com/onsi/gomega"
)

func (s *FeatureSuite) TestInstallationSuccessful() {
	g := NewWithT(s.T())

	s.testInstallation.Assertions.AssertInstallationWasSuccessful(g, s.ctx)
}

func (s *FeatureSuite) TestFailureAllowed() {
	g := NewWithT(s.T())

	g.Expect(1).NotTo(Equal(2), "1 does not equal 2")
}
