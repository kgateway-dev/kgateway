package helpers

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/solo-io/gloo/pkg/cliutil/install"
	"github.com/solo-io/skv2/codegen/util"
)

var dumpCommands = func(namespace string) []string {
	return []string{
		fmt.Sprintf("echo PODS FROM %s: && kubectl get pod -n %s --no-headers -o custom-columns=:metadata.name", namespace, namespace),
		fmt.Sprintf("for i in $(kubectl get pod -n %s --no-headers -o custom-columns=:metadata.name); do echo STATUS FOR %s.$i: $(kubectl get pod -n %s $i -o go-template=\"{{range .status.containerStatuses}}{{.state}}{{end}}\"); done", namespace, namespace, namespace),
		fmt.Sprintf("for i in $(kubectl get pod -n %s --no-headers -o custom-columns=:metadata.name); do echo LOGS FROM %s.$i: $(kubectl logs -n %s $i --all-containers); done", namespace, namespace, namespace),
		fmt.Sprintf("kubectl get events -n %s", namespace),
	}
}

func KubeDumpOnFail(out io.Writer, namespaces ...string) func() {
	return func() {
		outDir := setupOutDir()
		recordKubeState(fileAtPath(outDir + "kube-state.log"))
		recordDockerState(fileAtPath(outDir + "docker-state.log"))
		recordProcessState(fileAtPath(outDir + "process-state.log"))
		recordKubeDump(fileAtPath(outDir+"kube-dump.log"), namespaces...)
	}
}

func recordKubeDump(f *os.File, namespaces ...string) {
	defer f.Close()

	b := &bytes.Buffer{}
	b.WriteString("** Begin Kubernetes Dump ** \n")
	for _, ns := range namespaces {
		for _, command := range dumpCommands(ns) {
			cmd := exec.Command("bash", "-c", command)
			cmd.Stdout = b
			cmd.Stderr = b
			if err := cmd.Run(); err != nil {
				b.WriteString(fmt.Sprintf(
					"command %s failed: %v", command, b.String(),
				))
			}
		}
	}
	b.WriteString("** End Kubernetes Dump ** \n")
	f.WriteString(b.String())
}

func recordKubeState(f *os.File) {
	defer f.Close()

	kubeCli := &install.CmdKubectl{}
	kubeState, err := kubeCli.KubectlOut(nil, "get", "all", "-A")
	if err != nil {
		f.WriteString("*** Unable to get kube state ***\n")
		return
	}
	kubeEndpointsState, err := kubeCli.KubectlOut(nil, "get", "endpoints", "-A")
	if err != nil {
		f.WriteString("*** Unable to get kube state ***\n")
		return
	}
	f.WriteString("*** Kube state ***\n")
	f.WriteString(string(kubeState) + "\n")
	f.WriteString(string(kubeEndpointsState) + "\n")
	f.WriteString("*** End Kube state ***\n")
}

func recordDockerState(f *os.File) {
	defer f.Close()

	dockerCmd := exec.Command("docker", "ps")

	dockerState := &bytes.Buffer{}

	dockerCmd.Stdout = dockerState
	dockerCmd.Stderr = dockerState
	err := dockerCmd.Run()
	if err != nil {
		f.WriteString("*** Unable to get docker state ***\n")
		return
	}
	f.WriteString("*** Docker state ***\n")
	f.WriteString(dockerState.String() + "\n")
	f.WriteString("*** End Docker state ***\n")
}

func recordProcessState(f *os.File) {
	defer f.Close()

	psCmd := exec.Command("ps", "-auxf")

	psState := &bytes.Buffer{}

	psCmd.Stdout = psState
	psCmd.Stderr = psState
	err := psCmd.Run()
	if err != nil {
		f.WriteString("unable to get process state\n")
		return
	}
	f.WriteString("*** Process state ***\n")
	f.WriteString(psState.String() + "\n")
	f.WriteString("*** End Process state ***\n")
}

// setupOutDir forcibly deletes/creates the output directory, then return the path to it
func setupOutDir() string {
	outDir := filepath.Join(util.GetModuleRoot(), "_output", "test-failure-dump")

	err := os.RemoveAll(outDir)
	if err != nil {
		fmt.Printf("error removing log directory: %f\n", err)
	}
	err = os.MkdirAll(outDir, os.ModePerm)
	if err != nil {
		fmt.Printf("error creating log directory: %f\n", err)
	}

	fmt.Println("kube dump artifacts will be stored at: " + outDir)
	return outDir
}

// fileAtPath creates a file at the given path, and returns the file object
func fileAtPath(path string) *os.File {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		fmt.Printf("unable to openfile: %f\n", err)
	}
	return f
}
