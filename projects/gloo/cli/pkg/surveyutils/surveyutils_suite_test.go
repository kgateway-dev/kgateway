package surveyutils_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestSurveyUtils(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SurveyUtils Suite")
}
