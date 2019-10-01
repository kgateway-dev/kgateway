package k8sadmisssion

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httputil"

	"github.com/solo-io/solo-kit/pkg/utils/protoutils"

	"github.com/solo-io/gloo/pkg/utils"

	"go.opencensus.io/stats"
	"go.opencensus.io/tag"

	validationutil "github.com/solo-io/gloo/projects/gloo/pkg/utils/validation"

	validationapi "github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"

	"github.com/pkg/errors"
	gwv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	v2 "github.com/solo-io/gloo/projects/gateway/pkg/api/v2"
	"github.com/solo-io/gloo/projects/gateway/pkg/validation"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

const (
	ValidationPath      = "/validation"
	SkipValidationKey   = "gateway.solo.io/skip_validation"
	SkipValidationValue = "true"
)

var (
	runtimeScheme = runtime.NewScheme()
	codecs        = serializer.NewCodecFactory(runtimeScheme)
	deserializer  = codecs.UniversalDeserializer()

	resourceTypeKey, _ = tag.NewKey("resource_type")
	resourceRefKey, _  = tag.NewKey("resource_ref")

	mGatewayResourcesAccepted = utils.MakeCounter("validation.gateway.solo.io/resources_accepted", "The number of resources accepted")
	mGatewayResourcesRejected = utils.MakeCounter("validation.gateway.solo.io/resources_rejected", "The number of resources rejected")
)

func incrementMetric(ctx context.Context, resource string, ref core.ResourceRef, m *stats.Int64Measure) {
	utils.Increment(
		ctx,
		m,
		tag.Insert(resourceTypeKey, resource),
		tag.Insert(resourceRefKey, fmt.Sprintf("%v.%v", ref.Namespace, ref.Name)),
	)
}

func skipValidationCheck(annotations map[string]string) bool {
	if annotations == nil {
		return false
	}
	return annotations[SkipValidationKey] == SkipValidationValue
}

type WebhookConfig struct {
	ctx                           context.Context
	validator                     validation.Validator
	watchNamespaces               []string
	port                          int
	serverCertPath, serverKeyPath string
	alwaysAccept                  bool // accept all resources
}

func NewWebhookConfig(ctx context.Context, validator validation.Validator, watchNamespaces []string, port int, serverCertPath string, serverKeyPath string, alwaysAccept bool) WebhookConfig {
	return WebhookConfig{ctx: ctx, validator: validator, watchNamespaces: watchNamespaces, port: port, serverCertPath: serverCertPath, serverKeyPath: serverKeyPath, alwaysAccept: alwaysAccept}
}

func NewGatewayValidatingWebhook(cfg WebhookConfig) (*http.Server, error) {
	ctx := cfg.ctx
	validator := cfg.validator
	watchNamespaces := cfg.watchNamespaces
	port := cfg.port
	serverCertPath := cfg.serverCertPath
	serverKeyPath := cfg.serverKeyPath
	alwaysAccept := cfg.alwaysAccept

	keyPair, err := tls.LoadX509KeyPair(serverCertPath, serverKeyPath)
	if err != nil {
		return nil, errors.Wrapf(err, "loading x509 key pair")
	}

	handler := NewGatewayValidationHandler(
		contextutils.WithLogger(ctx, "gateway-validation-webhook"),
		validator,
		watchNamespaces,
		alwaysAccept,
	)

	mux := http.NewServeMux()
	mux.Handle(ValidationPath, handler)

	return &http.Server{
		Addr:      fmt.Sprintf(":%v", port),
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{keyPair}},
		Handler:   mux,
	}, nil

}

type gatewayValidationWebhook struct {
	ctx             context.Context
	validator       validation.Validator
	watchNamespaces []string
	alwaysAccept    bool
}

func NewGatewayValidationHandler(ctx context.Context, validator validation.Validator, watchNamespaces []string, alwaysAccept bool) *gatewayValidationWebhook {
	return &gatewayValidationWebhook{ctx: ctx, validator: validator, watchNamespaces: watchNamespaces, alwaysAccept: alwaysAccept}
}

func (wh *gatewayValidationWebhook) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	logger := contextutils.LoggerFrom(wh.ctx)

	logger.Infow("received validation request")

	b, _ := httputil.DumpRequest(r, true)
	logger.Debugf("validation request dump:\n %s", string(b))

	// Verify the content type is accurate
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		logger.Errorf("contentType=%s, expecting application/json", contentType)
		http.Error(w, "empty body", http.StatusBadRequest)
		return
	}

	var body []byte
	if r.Body != nil {
		if data, err := ioutil.ReadAll(r.Body); err == nil {
			body = data
		}
		defer r.Body.Close()
	}
	if len(body) == 0 {
		logger.Errorf("empty body")
		http.Error(w, "empty body", http.StatusBadRequest)
		return
	}

	var (
		admissionResponse *v1beta1.AdmissionResponse
		review            v1beta1.AdmissionReview
	)
	if _, _, err := deserializer.Decode(body, nil, &review); err == nil {
		admissionResponse = wh.makeAdmissionResponse(wh.ctx, &review)
	} else {
		logger.Errorf("Can't decode body: %v", err)
		admissionResponse = &v1beta1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	admissionReview := v1beta1.AdmissionReview{}
	if admissionResponse != nil {
		admissionReview.Response = admissionResponse
		if review.Request != nil {
			admissionReview.Response.UID = review.Request.UID
		}
	}

	resp, err := json.Marshal(admissionReview)
	if err != nil {
		logger.Errorf("Can't encode response: %v", err)
		http.Error(w, fmt.Sprintf("could not encode response: %v", err), http.StatusInternalServerError)
		return
	}
	logger.Infof("Ready to write response ...")
	if _, err := w.Write(resp); err != nil {
		logger.Errorf("Can't write response: %v", err)
		http.Error(w, fmt.Sprintf("could not write response: %v", err), http.StatusInternalServerError)
	}

	logger.Infof("responded with review: %s", resp)
}

func (wh *gatewayValidationWebhook) makeAdmissionResponse(ctx context.Context, review *v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
	logger := contextutils.LoggerFrom(ctx)

	req := review.Request

	logger.Infof("AdmissionReview for Kind=%v, Namespace=%v Name=%v UID=%v patchOperation=%v UserInfo=%v",
		req.Kind, req.Namespace, req.Name, req.UID, req.Operation, req.UserInfo)

	// ensure the request applies to a watched namespace, if watchNamespaces is set
	var validatingForNamespace bool
	if len(wh.watchNamespaces) > 0 {
		for _, ns := range wh.watchNamespaces {
			if ns == metav1.NamespaceAll || ns == req.Namespace {
				validatingForNamespace = true
				break
			}
		}
	} else {
		validatingForNamespace = true
	}

	// if it's not our namespace, do not validate
	if !validatingForNamespace {
		return &v1beta1.AdmissionResponse{
			Allowed: true,
		}
	}

	gvk := schema.GroupVersionKind{
		Group:   req.Kind.Group,
		Version: req.Kind.Version,
		Kind:    req.Kind.Kind,
	}

	ref := core.ResourceRef{
		Namespace: req.Namespace,
		Name:      req.Name,
	}

	isDelete := req.Operation == v1beta1.Delete

	proxyReports, validationErr := wh.validate(ctx, gvk, ref, req.Object, isDelete)

	success := &v1beta1.AdmissionResponse{
		Allowed: true,
	}

	if validationErr == nil {
		logger.Debug("Succeeded")

		incrementMetric(ctx, gvk.String(), ref, mGatewayResourcesAccepted)

		return success
	}

	incrementMetric(ctx, gvk.String(), ref, mGatewayResourcesRejected)

	logger.Errorf("Validation failed: %v", validationErr)

	if len(proxyReports) > 0 {
		var proxyErrs []error
		for _, rpt := range proxyReports {
			err := validationutil.GetProxyError(rpt)
			if err != nil {
				proxyErrs = append(proxyErrs, err)
			}
		}
		validationErr = errors.Errorf("resource incompatible with current Gloo snapshot: %v", proxyErrs)
	}

	if wh.alwaysAccept {
		return success
	}

	details := &metav1.StatusDetails{
		Name:   req.Name,
		Group:  gvk.Group,
		Kind:   gvk.Kind,
		Causes: wh.getFailureCauses(proxyReports),
	}

	return &v1beta1.AdmissionResponse{
		Result: &metav1.Status{
			Message: validationErr.Error(),
			Details: details,
		},
	}
}

func (wh *gatewayValidationWebhook) getFailureCauses(proxyReports validation.ProxyReports) []metav1.StatusCause {
	var causes []metav1.StatusCause
	for _, proxyReport := range proxyReports {
		for _, listenerReport := range proxyReport.ListenerReports {
			for _, err := range listenerReport.Errors {
				causes = append(causes, metav1.StatusCause{
					Message: fmt.Sprintf("Listener Error %v: %v", err.Type.String(), err.Reason),
				})
			}
			switch listener := listenerReport.ListenerTypeReport.(type) {
			case *validationapi.ListenerReport_HttpListenerReport:
				for _, err := range listener.HttpListenerReport.Errors {
					causes = append(causes, metav1.StatusCause{
						Message: fmt.Sprintf("HTTPListener Error %v: %v", err.Type.String(), err.Reason),
					})
				}
				for _, vh := range listener.HttpListenerReport.VirtualHostReports {
					for _, err := range vh.Errors {
						causes = append(causes, metav1.StatusCause{
							Message: fmt.Sprintf("VirtualHost Error %v: %v", err.Type.String(), err.Reason),
						})
					}
					for _, r := range vh.RouteReports {
						for _, err := range r.Errors {
							causes = append(causes, metav1.StatusCause{
								Message: fmt.Sprintf("Route Error %v: %v", err.Type.String(), err.Reason),
							})
						}
					}
				}
			case *validationapi.ListenerReport_TcpListenerReport:
				for _, err := range listener.TcpListenerReport.Errors {
					causes = append(causes, metav1.StatusCause{
						Message: fmt.Sprintf("TCPListener Error %v: %v", err.Type.String(), err.Reason),
					})
				}
				for _, host := range listener.TcpListenerReport.TcpHostReports {
					for _, err := range host.Errors {
						causes = append(causes, metav1.StatusCause{
							Message: fmt.Sprintf("TcpHost Error %v: %v", err.Type.String(), err.Reason),
						})
					}
				}
			}
		}
	}

	return causes
}

func (wh *gatewayValidationWebhook) validate(ctx context.Context, gvk schema.GroupVersionKind, ref core.ResourceRef, object runtime.RawExtension, isDelete bool) (validation.ProxyReports, error) {

	switch gvk {
	case v2.GatewayGVK:
		if isDelete {
			// we don't validate gateway deletion
			break
		}
		return wh.validateGateway(ctx, object.Raw)
	case gwv1.VirtualServiceGVK:
		if isDelete {
			return validation.ProxyReports{}, wh.validator.ValidateDeleteVirtualService(ctx, ref)
		} else {
			return wh.validateVirtualService(ctx, object.Raw)
		}
	case gwv1.RouteTableGVK:
		if isDelete {
			return validation.ProxyReports{}, wh.validator.ValidateDeleteRouteTable(ctx, ref)
		} else {
			return wh.validateRouteTable(ctx, object.Raw)
		}
	}
	return validation.ProxyReports{}, nil

}

func (wh *gatewayValidationWebhook) validateGateway(ctx context.Context, rawJson []byte) (validation.ProxyReports, error) {
	var gw v2.Gateway
	if err := protoutils.UnmarshalResource(rawJson, &gw); err != nil {
		return nil, errors.Wrapf(err, "could not unmarshal raw object")
	}
	if skipValidationCheck(gw.Metadata.Annotations) {
		return nil, nil
	}
	if proxyReports, err := wh.validator.ValidateGateway(ctx, &gw); err != nil {
		return proxyReports, errors.Wrapf(err, "Validating %T failed", gw)
	}
	return nil, nil
}

func (wh *gatewayValidationWebhook) validateVirtualService(ctx context.Context, rawJson []byte) (validation.ProxyReports, error) {
	var vs gwv1.VirtualService
	if err := protoutils.UnmarshalResource(rawJson, &vs); err != nil {
		return nil, errors.Wrapf(err, "could not unmarshal raw object")
	}
	if skipValidationCheck(vs.Metadata.Annotations) {
		return nil, nil
	}
	if proxyReports, err := wh.validator.ValidateVirtualService(ctx, &vs); err != nil {
		return proxyReports, errors.Wrapf(err, "Validating %T failed", vs)
	}
	return nil, nil
}

func (wh *gatewayValidationWebhook) validateRouteTable(ctx context.Context, rawJson []byte) (validation.ProxyReports, error) {
	var rt gwv1.RouteTable
	if err := protoutils.UnmarshalResource(rawJson, &rt); err != nil {
		return nil, errors.Wrapf(err, "could not unmarshal raw object")
	}
	if skipValidationCheck(rt.Metadata.Annotations) {
		return nil, nil
	}
	if proxyReports, err := wh.validator.ValidateRouteTable(ctx, &rt); err != nil {
		return proxyReports, errors.Wrapf(err, "Validating %T failed", rt)
	}
	return nil, nil
}
