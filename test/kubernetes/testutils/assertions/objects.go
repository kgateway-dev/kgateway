package assertions

import (
	"context"
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/solo-kit/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (p *Provider) EventuallyObjectsExist(ctx context.Context, objects ...client.Object) {
	for _, o := range objects {
		p.Gomega.Eventually(ctx, func(innerG Gomega) {
			err := p.clusterContext.Client.Get(ctx, client.ObjectKeyFromObject(o), o)
			innerG.Expect(err).NotTo(HaveOccurred(), "object %s %s should be available in cluster", o.GetObjectKind().GroupVersionKind().String(), client.ObjectKeyFromObject(o).String())
		}).
			WithContext(ctx).
			WithTimeout(time.Second*20).
			WithPolling(time.Millisecond*200).
			Should(Succeed(), fmt.Sprintf("object %s %s should be available in cluster", o.GetObjectKind().GroupVersionKind().String(), client.ObjectKeyFromObject(o).String()))
	}
}

func (p *Provider) EventuallyObjectsNotExist(ctx context.Context, objects ...client.Object) {
	for _, o := range objects {
		p.Gomega.Eventually(ctx, func(innerG Gomega) {
			err := p.clusterContext.Client.Get(ctx, client.ObjectKeyFromObject(o), o)
			innerG.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "object %s %s should not be found in cluster", o.GetObjectKind().GroupVersionKind().String(), client.ObjectKeyFromObject(o).String())
		}).
			WithContext(ctx).
			WithTimeout(time.Second*20).
			WithPolling(time.Millisecond*200).
			Should(Succeed(), fmt.Sprintf("object %s %s should not be found in cluster", o.GetObjectKind().GroupVersionKind().String(), client.ObjectKeyFromObject(o).String()))
	}
}

func (p *Provider) ConsistentlyObjectsNotExist(ctx context.Context, objects ...client.Object) {
	for _, o := range objects {
		p.Gomega.Consistently(ctx, func(innerG Gomega) {
			err := p.clusterContext.Client.Get(ctx, client.ObjectKeyFromObject(o), o)
			innerG.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "object %s %s should not be found in cluster", o.GetObjectKind().GroupVersionKind().String(), client.ObjectKeyFromObject(o).String())
		}).
			WithContext(ctx).
			WithTimeout(time.Second*10).
			WithPolling(time.Second*1).
			Should(Succeed(), fmt.Sprintf("object %s %s should not be found in cluster", o.GetObjectKind().GroupVersionKind().String(), client.ObjectKeyFromObject(o).String()))
	}
}

func (p *Provider) ExpectNamespaceNotExist(ctx context.Context, ns string) {
	_, err := p.clusterContext.Clientset.CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{})
	p.Gomega.Expect(apierrors.IsNotFound(err)).To(BeTrue(), fmt.Sprintf("namespace %s should not be found in cluster", ns))
}

func (p *Provider) ExpectGlooObjectNotExist(ctx context.Context, getter helpers.InputResourceGetter, meta *metav1.ObjectMeta) {
	_, err := getter()
	p.Gomega.Expect(errors.IsNotExist(err)).To(BeTrue(), fmt.Sprintf("obj %s.%s should not be found in cluster", meta.GetName(), meta.GetNamespace()))
}

// ExpectObjectAdmitted should be used when applying Policy objects that are subject to the Gloo Gateway Validation Webhook
// If the testInstallation has validation enabled and the manifest contains a known substring (e.g. `webhook-reject`)
// we expect the application to fail, with an expected error substring supplied as `expectedOutput`
func (p *Provider) ExpectObjectAdmitted(manifest string, err error, actualOutput, expectedOutput string) {
	if p.glooGatewayContext.ValidationAlwaysAccept {
		p.Assert.NoError(err, "can apply "+manifest)
		return
	}

	if strings.Contains(manifest, WebhookReject) {
		// when validation is enforced (i.e. does NOT always accept), an apply should result in an error
		// and the output from the command should contain a validation failure message
		p.Assert.Error(err, "got error when applying "+manifest)
		p.Assert.Contains(actualOutput, expectedOutput, "apply failed with expected message for "+manifest)
	} else {
		p.Assert.NoError(err, "can apply "+manifest)
	}
}

func (p *Provider) ExpectObjectDeleted(manifest string, err error, actualOutput string) {
	if p.glooGatewayContext.ValidationAlwaysAccept {
		p.Assert.NoError(err, "can delete "+manifest)
		return
	}

	if strings.Contains(manifest, WebhookReject) {
		// when validation is enforced (i.e. does NOT always accept), a delete should result in an error and "not found" in the output
		p.Assert.Error(err, "delete failed for "+manifest)
		p.Assert.Contains(actualOutput, "NotFound")
	} else {
		p.Assert.NoError(err, "can delete "+manifest)
	}
}
