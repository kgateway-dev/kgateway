package surveyutils_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestSurveyutils(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Surveyutils Suite")
}
