package helper

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientsv1 "k8s.io/client-go/kubernetes/typed/apps/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func (h *SoloTestHelper) ModifyDeploymentEnv(ctx context.Context, deploymentClient clientsv1.DeploymentInterface, namespace string, deploymentName string, containerIndex int, envVar corev1.EnvVar) {
	GinkgoHelper()

	d, err := deploymentClient.Get(ctx, deploymentName, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())

	// make sure we are referencing a valid container
	Expect(len(d.Spec.Template.Spec.Containers)).To(BeNumerically(">", containerIndex))

	// if an env var with the given name already exists, modify it
	exists := false
	for i, env := range d.Spec.Template.Spec.Containers[containerIndex].Env {
		if env.Name == envVar.Name {
			d.Spec.Template.Spec.Containers[containerIndex].Env[i].Value = envVar.Value
			exists = true
			break
		}
	}
	// otherwise add a new env var
	if !exists {
		d.Spec.Template.Spec.Containers[containerIndex].Env = append(d.Spec.Template.Spec.Containers[containerIndex].Env, envVar)
	}
	_, err = deploymentClient.Update(ctx, d, metav1.UpdateOptions{})
	Expect(err).NotTo(HaveOccurred())

	h.WaitForRollout(ctx, deploymentName, namespace, "60s", "1s")
}

// WaitForRollout waits for the specified deployment to be rolled out successfully.
func (h *SoloTestHelper) WaitForRollout(ctx context.Context, deploymentName string, deploymentNamespace string, intervals ...interface{}) {
	GinkgoHelper()

	Eventually(func() (bool, error) {
		out, _, err := h.Cli.Execute(ctx, "rollout", "status", "-n", deploymentNamespace, fmt.Sprintf("deployment/%s", deploymentName))
		return strings.Contains(out, "successfully rolled out"), err
	}, "30s", "1s").Should(BeTrue())
}

// Can be replaced entirely with Cli
func (h *SoloTestHelper) GetContainerLogs(ctx context.Context, namespace string, name string) string {
	GinkgoHelper()

	out, err := h.Cli.GetContainerLogs(ctx, namespace, name)
	Expect(err).ToNot(HaveOccurred())
	return out
}
