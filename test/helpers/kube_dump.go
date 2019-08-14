package helpers

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"

	"github.com/pkg/errors"
)

var dumpCommands = func(namespace string) []string {
	return []string{
		fmt.Sprintf("echo PODS FROM %s && kubectl get pod -n %s", namespace, namespace),
		fmt.Sprintf("for i in $(kubectl get pod -n %s --no-headers -o custom-columns=:metadata.name); do echo LOGS FROM %s.$i $(kubectl logs -n %s $i); done", namespace, namespace, namespace),
		fmt.Sprintf("for i in $(kubectl get pod -n %s --no-headers -o custom-columns=:metadata.name); do echo STATUS FOR %s.$i $(kubectl pod -n %s $i -o go-template=\"{{range .status.containerStatuses}}{{.state}}{{end}}\"); done", namespace, namespace, namespace),
	}
}

// dump all data from the kube cluster
func KubeDump(namespaces ...string) (string, error) {
	b := &bytes.Buffer{}
	b.WriteString("Complete Kubernetes Dump\n")
	for _, ns := range namespaces {
		for _, command := range dumpCommands(ns) {
			cmd := exec.Command("bash", "-c", command)
			cmd.Stdout = b
			cmd.Stderr = b
			if err := cmd.Run(); err != nil {
				return "", errors.Errorf("command %v failed: %v", command, b.String())
			}
		}
	}
	return b.String(), nil
}

func KubeDumpOnFail(out io.Writer, namespaces ...string) func() {
	return func() {
		dump, err := KubeDump(namespaces...)
		if err != nil {
			fmt.Fprintf(out, "getting dump failed: %v", err)
		}
		fmt.Fprintf(out, dump)
	}
}
