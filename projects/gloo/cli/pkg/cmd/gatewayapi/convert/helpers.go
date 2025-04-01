package convert

import (
	"bytes"
	"strings"

	"golang.org/x/exp/rand"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"

	"github.com/ghodss/yaml"

	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyz")
var r = rand.New(rand.NewSource(RandomSeed))

func RandStringRunes(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[r.Intn(len(letterRunes))]
	}
	return string(b)
}

func convertDomains(domains []string) []gwv1.Hostname {

	var hostnames []gwv1.Hostname
	for _, d := range domains {
		if strings.Contains(d, ":") {
			//skip all hostnames with ports listed (this is caught and logged in the virtualservice to listener set part)
			continue
		}
		hostnames = append(hostnames, gwv1.Hostname(d))
	}
	return hostnames
}

type YamlMarshaller struct{}

func (YamlMarshaller) ToYaml(resource interface{}) ([]byte, error) {
	switch typedResource := resource.(type) {
	case nil:
		return []byte{}, nil
	case proto.Message:
		buf := &bytes.Buffer{}
		if err := (&jsonpb.Marshaler{OrigName: true}).Marshal(buf, typedResource); err != nil {
			return nil, err
		}
		return yaml.JSONToYAML(buf.Bytes())
	default:
		return yaml.Marshal(resource)
	}
}
