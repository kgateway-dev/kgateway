package helpers

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/solo-io/gloo/pkg/cliutil/install"
	"github.com/solo-io/skv2/codegen/util"
)

var (
	outDir = filepath.Join(util.GetModuleRoot(), "_output", "kube2e-artifacts")
)

// KubeDumpOnFail creates a small dump of the kubernetes state when a test fails.
// This is useful for debugging test failures.
// The dump is written to _output/kube2e-artifacts.
// The dump includes:
// - docker state
// - process state
// - kubernetes state
// - logs from all pods in the given namespaces
// - yaml representations of all solo.io CRs in the given namespaces
func KubeDumpOnFail(out io.Writer, namespaces ...string) func() {
	return func() {
		setupOutDir()

		recordDockerState(fileAtPath(filepath.Join(outDir, "docker-state.log")))
		recordProcessState(fileAtPath(filepath.Join(outDir, "process-state.log")))
		recordKubeState(fileAtPath(filepath.Join(outDir, "kube-state.log")))

		recordKubeDump(namespaces...)
	}
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
		f.WriteString("*** Unable to get endpoint state ***\n")
		return
	}
	kubePeerAuthenticationState, err := kubeCli.KubectlOut(nil, "get", "peerauthentications.security.istio.io", "-A")
	if err != nil {
		f.WriteString("*** Unable to get peerauthentication state ***\n")
		return
	}
	f.WriteString("*** Kube state ***\n")
	f.WriteString(string(kubeState) + "\n")
	f.WriteString(string(kubeEndpointsState) + "\n")
	f.WriteString(string(kubePeerAuthenticationState) + "\n")
	f.WriteString("*** End Kube state ***\n")

	podSpecs, err := kubeCli.KubectlOut(nil, "get", "pods", "-n", "gloo-system", "-oyaml")
	if err != nil {
		f.WriteString("*** Unable to get pod specs ***\n")
		return
	}
	f.WriteString("*** Pod specs ***\n")
	f.WriteString(string(podSpecs) + "\n")
	f.WriteString("*** End Pod specs ***\n")
}

func recordKubeDump(namespaces ...string) {
	// for each namespace, create a namespace directory that contains...
	for _, ns := range namespaces {
		// ...a pod logs subdirectoy
		if err := recordPods(filepath.Join(outDir, ns, "_pods"), ns); err != nil {
			fmt.Printf("error recording pod logs: %f, \n", err)
		}

		// ...and a subdirectory for each solo.io CRD with non-zero resources
		if err := recordCRs(filepath.Join(outDir, ns), ns); err != nil {
			fmt.Printf("error recording pod logs: %f, \n", err)
		}
	}
}

// recordPods records logs from each pod to _output/kube2e-artifacts/$namespace/pods/$pod.log
func recordPods(podDir, namespace string) error {
	pods, err := kubeList(namespace, "pod")
	if err != nil {
		return err
	}

	for _, pod := range pods {
		if err := os.MkdirAll(podDir, os.ModePerm); err != nil {
			return err
		}

		f := fileAtPath(filepath.Join(podDir, pod+".log"))
		logs, err := kubeLogs(namespace, pod)
		if err != nil {
			return err
		}
		f.WriteString(logs)
		f.Close()
	}
	return nil
}

// recordCRs records all unique CRs floating about to _output/kube2e-artifacts/$namespace/$crd/$cr.yaml
func recordCRs(namespaceDir string, namespace string) error {
	crds, err := kubeList(namespace, "crd")
	if err != nil {
		return err
	}

	// record all unique CRs floating about
	for _, crd := range crds {
		// consider all installed CRDs that are solo-managed
		if !strings.Contains(crd, "solo.io") {
			continue
		}

		// if there are any existing CRs corresponding to this CRD
		crs, err := kubeList(namespace, crd)
		if err != nil {
			return err
		}
		if len(crs) == 0 {
			continue
		}
		crdDir := filepath.Join(namespaceDir, crd)
		if err := os.MkdirAll(crdDir, os.ModePerm); err != nil {
			return err
		}

		// we record each one in its own .yaml representation
		for _, cr := range crs {
			f := fileAtPath(filepath.Join(crdDir, cr+".yaml"))
			crDetails, err := kubeGet(namespace, crd, cr)
			if err != nil {
				return err
			}
			f.WriteString(crDetails)
			f.Close()
		}
	}

	return nil
}

// kubeLogs runs $(kubectl -n $namespace logs $pod --all-containers) and returns the string result
func kubeLogs(namespace string, pod string) (string, error) {
	kubeCli := &install.CmdKubectl{}
	toReturn, err := kubeCli.KubectlOut(nil, "-n", namespace, "logs", pod, "--all-containers")
	return string(toReturn), err
}

// kubeGet runs $(kubectl -n $namespace get $kubeType $name -oyaml) and returns the string result
func kubeGet(namespace string, kubeType string, name string) (string, error) {
	kubeCli := &install.CmdKubectl{}
	toReturn, err := kubeCli.KubectlOut(nil, "-n", namespace, "get", kubeType, name, "-oyaml")
	return string(toReturn), err
}

// kubeList runs $(kubectl -n $namespace $target) and returns a slice of kubernetes object names
func kubeList(namespace string, target string) ([]string, error) {
	kubeCli := &install.CmdKubectl{}
	line, err := kubeCli.KubectlOut(nil, "-n", namespace, "get", target)
	if err != nil {
		return nil, err
	}

	var toReturn []string
	for _, line := range strings.Split(strings.TrimSuffix(string(line), "\n"), "\n") {
		if strings.HasPrefix(line, "NAME") || strings.HasPrefix(line, "No resources found") {
			continue // skip header line and cases where there are no resources
		}
		if split := strings.Split(line, " "); len(split) > 1 {
			toReturn = append(toReturn, split[0])
		}
	}
	return toReturn, nil
}

// setupOutDir forcibly deletes/creates the output directory
func setupOutDir() {
	err := os.RemoveAll(outDir)
	if err != nil {
		fmt.Printf("error removing log directory: %f\n", err)
	}
	err = os.MkdirAll(outDir, os.ModePerm)
	if err != nil {
		fmt.Printf("error creating log directory: %f\n", err)
	}

	fmt.Println("kube dump artifacts will be stored at: " + outDir)
}

// fileAtPath creates a file at the given path, and returns the file object
func fileAtPath(path string) *os.File {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		fmt.Printf("unable to openfile: %f\n", err)
	}
	return f
}
