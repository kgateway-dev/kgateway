package cliutil

import (
	survey "gopkg.in/AlecAivazis/survey.v1"
	"gopkg.in/AlecAivazis/survey.v1/terminal"
)

func UseStdio(io terminal.Stdio) {
	stdio = &io
}

var stdio *terminal.Stdio

func AskOne(p survey.Prompt, response interface{}, v survey.Validator, opts ...survey.AskOpt) error {
	if stdio != nil {
		opts = append(opts, survey.WithStdio(stdio.In, stdio.Out, stdio.Err))
	}
	return survey.AskOne(p, response, v, opts...)
}
