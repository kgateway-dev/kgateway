package install

import (
	"bytes"
	"text/template"
)

const (
	IngressValues = `
gateway:
  enabled: false
ingress:
  enabled: true
`
	KnativeValuesTemplate = `
gateway:
  enabled: false
settings:
  integrations:
    knative:
      enabled: true
      version: {{ . }}
`
)

func RenderKnativeValues(version string) (string, error) {
	parsedTemplate := template.Must(template.New("knativeValues").Parse(KnativeValuesTemplate))

	var b bytes.Buffer
	if err := parsedTemplate.Execute(&b, version); err != nil {
		return "", err
	}
	return b.String(), nil
}
