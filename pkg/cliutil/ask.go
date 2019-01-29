package cliutil

import (
	survey "gopkg.in/AlecAivazis/survey.v1"
)

func AskOne(p survey.Prompt, response interface{}, v survey.Validator, opts ...survey.AskOpt) error {
	return survey.AskOne(p, response, v, opts...)
}
