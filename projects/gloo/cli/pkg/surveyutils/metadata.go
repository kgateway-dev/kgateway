package surveyutils

import (
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	survey "gopkg.in/AlecAivazis/survey.v1"
)

var namespaceQuestion = &survey.Input{
	Message: "namespace:",
	Default: defaults.GlooSystem,
}

func NamespaceSurvey() string {
	var namespace string
	survey.AskOne(namespaceQuestion, &namespace, survey.Required)
	return namespace
}

var nameQuestion = &survey.Input{
	Message: "name:",
}

func NameSurvey() string {
	var podname string
	survey.AskOne(nameQuestion, &podname, survey.Required)
	return podname
}

func MetadataSurvey(metadata *core.Metadata) {
	qs := []*survey.Question{
		{
			Name:   "namespace",
			Prompt: namespaceQuestion,
		},
		{
			Name:   "name",
			Prompt: nameQuestion,
		},
	}
	survey.Ask(qs, metadata)
}
