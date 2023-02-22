package surveyutils_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSurveyUtils(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SurveyUtils Suite")
}
