package bootstrap

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"text/template"

	"github.com/solo-io/go-utils/contextutils"

	"github.com/golang/protobuf/proto"
	"github.com/solo-io/go-utils/protoutils"
	"sigs.k8s.io/yaml"

	"github.com/rotisserie/eris"
)

type EnvoyInstance struct {
	Transformations string
}

const envoyPath = "/usr/local/bin/envoy"

func ToYaml(m proto.Message) ([]byte, error) {
	jsn, err := protoutils.MarshalBytes(m)
	if err != nil {
		return nil, err
	}
	return yaml.JSONToYAML(jsn)
}

func IndentYaml(yml string, numSpaces int) string {
	lines := strings.Split(yml, "\n")
	indent := strings.Repeat(" ", numSpaces)
	indentedYaml := strings.Join(lines, "\n"+indent)
	return indentedYaml
}

func (ei *EnvoyInstance) ValidateBootstrap(ctx context.Context, bootstrapTemplate string) error {
	configYaml := ei.buildBootstrap(bootstrapTemplate)
	validateCmd := exec.Command(envoyPath, "--mode", "validate", "--config-yaml", configYaml)
	if output, err := validateCmd.CombinedOutput(); err != nil {
		if os.IsNotExist(err) {
			// log a warning and return nil; will allow users to continue to run Gloo locally without
			// relying on the Gloo container with Envoy already published to the expected directory
			contextutils.LoggerFrom(ctx).Warnf("Unable to validate envoy configuration using envoy at %v; "+
				"skipping additional validation of Gloo config.", envoyPath)
			return nil
		}
		return eris.Errorf("%v", string(output), err)
	}
	return nil
}

func (ei *EnvoyInstance) buildBootstrap(bootstrapTemplate string) string {
	var b bytes.Buffer
	parsedTemplate := template.Must(template.New("bootstrap").Parse(bootstrapTemplate))
	if err := parsedTemplate.Execute(&b, ei); err != nil {
		panic(err)
	}
	return b.String()
}

const TransformationBootstrapTemplate = `
node:
  cluster: doesntmatter
  id: imspecial
  metadata:
    role: "gloo-system~gateway-proxy"
static_resources:
  clusters:
  - name: placeholder_cluster
    connect_timeout: 5.000s
  listeners:
  - address:
      socket_address:
        address: 0.0.0.0
        port_value: 8081
    filter_chains:
    - filters:
      - config:
          route_config:
            name: placeholder_route
            virtual_hosts:
            - domains:
              - '*'
              name: placeholder_host
              routes:
              - match:
                  headers:
                  - exact_match: GET
                    name: :method
                  path: /
                route:
                  cluster: placeholder_cluster
              per_filter_config:
                io.solo.transformation:
                  {{.Transformations}}
          stat_prefix: placeholder
        name: envoy.http_connection_manager
    name: placeholder_listener
`
