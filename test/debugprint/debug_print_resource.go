package debugprint

import (
	"fmt"
	"log"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/onsi/ginkgo/v2"
	"github.com/solo-io/gloo/pkg/utils/protoutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/crd"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"sigs.k8s.io/yaml"
)

func PrintYaml(ress ...proto.Message) {
	log.Printf(SprintYaml(ress...))
}

func GinkgoPrintYaml(ress ...proto.Message) {
	fmt.Fprint(ginkgo.GinkgoWriter, SprintYaml(ress...))
}

func PrintKube(crd crd.Crd, ress ...resources.InputResource) error {
	for _, rs := range ress {
		res, err := crd.KubeResource(rs)
		if err != nil {
			return err
		}
		yam, err := yaml.Marshal(res)
		if err != nil {
			return err
		}
		log.Printf("%s", yam)
	}
	return nil
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
