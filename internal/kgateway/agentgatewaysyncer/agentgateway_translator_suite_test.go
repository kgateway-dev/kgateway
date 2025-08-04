package agentgatewaysyncer

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAgentgatewayTranslator(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "agentgateway Translator Suite")
}
