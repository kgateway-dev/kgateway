package helpers

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"

	"github.com/solo-io/gloo/pkg/cliutil/install"

	errors "github.com/rotisserie/eris"
)

var dumpCommands = func(namespace string) []string {
	return []string{
		fmt.Sprintf("echo PODS FROM %s: && kubectl get pod -n %s --no-headers -o custom-columns=:metadata.name", namespace, namespace),
		fmt.Sprintf("for i in $(kubectl get pod -n %s --no-headers -o custom-columns=:metadata.name); do echo STATUS FOR %s.$i: $(kubectl get pod -n %s $i -o go-template=\"{{range .status.containerStatuses}}{{.state}}{{end}}\"); done", namespace, namespace, namespace),
		fmt.Sprintf("for i in $(kubectl get pod -n %s --no-headers -o custom-columns=:metadata.name); do echo LOGS FROM %s.$i: $(kubectl logs -n %s $i --all-containers); done", namespace, namespace, namespace),
	}
}

// dump all data from the kube cluster
func KubeDump(namespaces ...string) (string, error) {
	b := &bytes.Buffer{}
	b.WriteString("** Begin Kubernetes Dump ** \n")
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
	b.WriteString("** End Kubernetes Dump ** \n")
	return b.String(), nil
}

func KubeDumpOnFail(out io.Writer, namespaces ...string) func() {
	return func() {
		PrintKubeState()
		PrintDockerState()
		PrintProcessState()
		dump, err := KubeDump(namespaces...)
		if err != nil {
			fmt.Fprintf(out, "getting kube dump failed: %v", err)
		}
		fmt.Fprintf(out, dump)
	}
}

func PrintKubeState() {
	kubeCli := &install.CmdKubectl{}
	kubeState, err := kubeCli.KubectlOut(nil, "get", "all", "-A")
	if err != nil {
		fmt.Println("*** Unable to get kube state ***")
		return
	}
	kubeEndpointsState, err := kubeCli.KubectlOut(nil, "get", "endpoints", "-A")
	if err != nil {
		fmt.Println("*** Unable to get kube state ***")
		return
	}
	fmt.Println("*** Kube state ***")
	fmt.Println(string(kubeState))
	fmt.Println(string(kubeEndpointsState))
	fmt.Println("*** End Kube state ***")
}

func PrintDockerState() {
	dockerCmd := exec.Command("docker", "ps")

	dockerState := &bytes.Buffer{}

	dockerCmd.Stdout = dockerState
	dockerCmd.Stderr = dockerState
	err := dockerCmd.Run()
	if err != nil {
		fmt.Println("*** Unable to get docker state ***")
		return
	}
	fmt.Println("*** Docker state ***")
	fmt.Println(dockerState.String())
	fmt.Println("*** End Docker state ***")
}

func PrintProcessState() {
	psCmd := exec.Command("ps", "-auxf")

	psState := &bytes.Buffer{}

	psCmd.Stdout = psState
	psCmd.Stderr = psState
	err := psCmd.Run()
	if err != nil {
		fmt.Println("*** Unable to get process state ***")
		return
	}
	fmt.Println("*** Process state ***")
	fmt.Println(psState.String())
	fmt.Println("*** End Process state ***")
}
