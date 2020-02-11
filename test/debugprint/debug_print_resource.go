package debugprint

import (
	"fmt"
	"log"
	"strings"

	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/crd"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"

	"github.com/gogo/protobuf/proto"
	"github.com/onsi/ginkgo"
	"github.com/solo-io/go-utils/protoutils"
	"sigs.k8s.io/yaml"
)

func PrintYaml(ress ...proto.Message) {
	log.Printf(SprintYaml(ress...))
}

func GinkgoPrintYaml(ress ...proto.Message) {
	fmt.Fprint(ginkgo.GinkgoWriter, SprintYaml(ress...))
}

func PrintKube(crd crd.Crd, ress ...resources.InputResource) {
	for _, rs := range ress {
		res := crd.KubeResource(rs)
		yam, _ := yaml.Marshal(res)
		log.Printf("%s", yam)
	}
}

func PrintAny(any ...interface{}) {
	for _, res := range any {
		yam, _ := yaml.Marshal(res)
		log.Printf("%s", yam)
	}
}

func SprintAny(any ...interface{}) string {
	var yams []string
	for _, res := range any {
		yam, _ := yaml.Marshal(res)
		yams = append(yams, string(yam))
	}
	return strings.Join(yams, "\n---\n")
}

func SprintYaml(ress ...proto.Message) string {
	var yams []string
	for _, res := range ress {
		js, _ := protoutils.MarshalBytes(res)
		yam, _ := yaml.JSONToYAML(js)

		yams = append(yams, string(yam))
	}
	return strings.Join(yams, "\n---\n")
}
