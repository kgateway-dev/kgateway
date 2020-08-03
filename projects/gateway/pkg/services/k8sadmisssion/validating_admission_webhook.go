package k8sadmisssion

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"

	"github.com/ghodss/yaml"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"

	"github.com/solo-io/solo-kit/pkg/utils/protoutils"

	"github.com/solo-io/gloo/pkg/utils"

	"go.opencensus.io/stats"
	"go.opencensus.io/tag"

	validationutil "github.com/solo-io/gloo/projects/gloo/pkg/utils/validation"

	validationapi "github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"

	errors "github.com/rotisserie/eris"
	gwv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
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

	mGatewayResourcesAccepted = utils.MakeSumCounter("validation.gateway.solo.io/resources_accepted", "The number of resources accepted")
	mGatewayResourcesRejected = utils.MakeSumCounter("validation.gateway.solo.io/resources_rejected", "The number of resources rejected")

	unmarshalErrMsg     = "could not unmarshal raw object"
	UnmarshalErr        = errors.New(unmarshalErrMsg)
	WrappedUnmarshalErr = func(err error) error {
		return errors.Wrapf(err, unmarshalErrMsg)
	}
	ListGVK = schema.GroupVersionKind{
		Version: "v1",
		Group:   "",
		Kind:    "List",
	}
)

const (
	ApplicationJson = "application/json"
	ApplicationYaml = "application/x-yaml"
)

func incrementMetric(ctx context.Context, resource string, ref core.ResourceRef, m *stats.Int64Measure) {
	utils.MeasureOne(
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
	readGatewaysFromAllNamespaces bool
	webhookNamespace              string
}

func NewWebhookConfig(ctx context.Context, validator validation.Validator, watchNamespaces []string, port int, serverCertPath, serverKeyPath string, alwaysAccept, readGatewaysFromAllNamespaces bool, webhookNamespace string) WebhookConfig {
	return WebhookConfig{
		ctx:                           ctx,
		validator:                     validator,
		watchNamespaces:               watchNamespaces,
		port:                          port,
		serverCertPath:                serverCertPath,
		serverKeyPath:                 serverKeyPath,
		alwaysAccept:                  alwaysAccept,
		readGatewaysFromAllNamespaces: readGatewaysFromAllNamespaces,
		webhookNamespace:              webhookNamespace}
}

func NewGatewayValidatingWebhook(cfg WebhookConfig) (*http.Server, error) {
	ctx := cfg.ctx
	validator := cfg.validator
	watchNamespaces := cfg.watchNamespaces
	port := cfg.port
	serverCertPath := cfg.serverCertPath
	serverKeyPath := cfg.serverKeyPath
	alwaysAccept := cfg.alwaysAccept
	readGatewaysFromAllNamespaces := cfg.readGatewaysFromAllNamespaces
	webhookNamespace := cfg.webhookNamespace

	keyPair, err := tls.LoadX509KeyPair(serverCertPath, serverKeyPath)
	if err != nil {
		return nil, errors.Wrapf(err, "loading x509 key pair")
	}

	handler := NewGatewayValidationHandler(
		contextutils.WithLogger(ctx, "gateway-validation-webhook"),
		validator,
		watchNamespaces,
		alwaysAccept,
		readGatewaysFromAllNamespaces,
		webhookNamespace,
	)

	mux := http.NewServeMux()
	mux.Handle(ValidationPath, handler)

	return &http.Server{
		Addr:      fmt.Sprintf(":%v", port),
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{keyPair}},
		Handler:   mux,
		ErrorLog:  log.New(&debugLogger{ctx: ctx}, "validation-webhook-server", log.LstdFlags),
	}, nil

}

type debugLogger struct{ ctx context.Context }

func (l *debugLogger) Write(p []byte) (n int, err error) {
	contextutils.LoggerFrom(l.ctx).Debug(string(p))
	return len(p), nil
}

type gatewayValidationWebhook struct {
	ctx                           context.Context
	validator                     validation.Validator
	watchNamespaces               []string
	alwaysAccept                  bool
	readGatewaysFromAllNamespaces bool
	webhookNamespace              string
}

type AdmissionReviewWithProxies struct {
	v1beta1.AdmissionReview
	Proxies []*gloov1.Proxy `json:"proxies,omitempty"`
}

// Validation webhook works properly even if extra fields are provided in the response
type AdmissionResponseWithProxies struct {
	*v1beta1.AdmissionResponse
	Proxies []*gloov1.Proxy `json:"proxies,omitempty"`
}

func NewGatewayValidationHandler(ctx context.Context, validator validation.Validator, watchNamespaces []string, alwaysAccept bool, readGatewaysFromAllNamespaces bool, webhookNamespace string) *gatewayValidationWebhook {
	return &gatewayValidationWebhook{ctx: ctx,
		validator:                     validator,
		watchNamespaces:               watchNamespaces,
		alwaysAccept:                  alwaysAccept,
		readGatewaysFromAllNamespaces: readGatewaysFromAllNamespaces,
		webhookNamespace:              webhookNamespace}
}

func (wh *gatewayValidationWebhook) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	logger := contextutils.LoggerFrom(wh.ctx)

	logger.Infow("received validation request")

	b, _ := httputil.DumpRequest(r, true)
	logger.Debugf("validation request dump:\n %s", string(b))

	// Verify the content type is accurate
	contentType := r.Header.Get("Content-Type")
	if contentType != ApplicationJson && contentType != ApplicationYaml {
		logger.Errorf("contentType=%s, expecting application/json or application/x-yaml", contentType)
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
		admissionResponse = &AdmissionResponseWithProxies{}
		review            v1beta1.AdmissionReview
		err               error
	)

	if contentType == ApplicationYaml {
		if err = yaml.Unmarshal(body, &review); err == nil {
			admissionResponse = wh.makeAdmissionResponse(wh.ctx, &review)
		}
	} else {
		if _, _, err := deserializer.Decode(body, nil, &review); err == nil {
			admissionResponse = wh.makeAdmissionResponse(wh.ctx, &review)
		}
	}

	if err != nil {
		logger.Errorf("Can't decode body: %v", err)
		admissionResponse.AdmissionResponse = &v1beta1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	admissionReview := AdmissionReviewWithProxies{}
	if admissionResponse != nil {
		admissionReview.Response = admissionResponse.AdmissionResponse
		admissionReview.Proxies = admissionResponse.Proxies
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

func (wh *gatewayValidationWebhook) makeAdmissionResponse(ctx context.Context, review *v1beta1.AdmissionReview) *AdmissionResponseWithProxies {
	logger := contextutils.LoggerFrom(ctx)

	req := review.Request

	logger.Infof("AdmissionReview for Kind=%v, Namespace=%v Name=%v UID=%v patchOperation=%v UserInfo=%v",
		req.Kind, req.Namespace, req.Name, req.UID, req.Operation, req.UserInfo)

	gvk := schema.GroupVersionKind{
		Group:   req.Kind.Group,
		Version: req.Kind.Version,
		Kind:    req.Kind.Kind,
	}

	// If we've specified to NOT read gateway requests from all namespaces, then only
	// check gateway requests for the same namespace as this webhook, regardless of the
	// contents of watchNamespaces. It's assumed that if it's non-empty, watchNamespaces
	// contains the webhook's own namespace, since this was checked during setup in setup_syncer.go
	watchNamespaces := wh.watchNamespaces
	if gvk == gwv1.GatewayGVK && !wh.readGatewaysFromAllNamespaces && !utils.AllNamespaces(wh.watchNamespaces) {
		watchNamespaces = []string{wh.webhookNamespace}
	}

	// ensure the request applies to a watched namespace, if watchNamespaces is set
	var validatingForNamespace bool
	if len(watchNamespaces) > 0 {
		for _, ns := range watchNamespaces {
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
		return &AdmissionResponseWithProxies{
			AdmissionResponse: &v1beta1.AdmissionResponse{
				Allowed: true,
			},
		}
	}

	ref := core.ResourceRef{
		Namespace: req.Namespace,
		Name:      req.Name,
	}

	isDelete := req.Operation == v1beta1.Delete

	var dryRun bool
	if req.DryRun != nil {
		dryRun = *req.DryRun
	}

	proxyReports, validationErr := wh.validate(ctx, gvk, ref, req.Object.Raw, isDelete, dryRun)
	var proxies []*gloov1.Proxy
	for proxy, _ := range proxyReports {
		proxies = append(proxies, proxy)
	}

	isUnmarshalErr := validationErr != nil && errors.Is(validationErr, UnmarshalErr)

	// even if validation is set to always accept, we want to fail on unmarshal errors
	if validationErr == nil || (wh.alwaysAccept && !isUnmarshalErr) {
		logger.Debug("Succeeded, alwaysAccept: %v validationErr: %v", wh.alwaysAccept, validationErr)
		incrementMetric(ctx, gvk.String(), ref, mGatewayResourcesAccepted)
		return &AdmissionResponseWithProxies{
			AdmissionResponse: &v1beta1.AdmissionResponse{
				Allowed: true,
			},
			Proxies: proxies,
		}
	}

	incrementMetric(ctx, gvk.String(), ref, mGatewayResourcesRejected)
	logger.Errorf("Validation failed: %v", validationErr)

	var proxyErrs []error
	for _, rpt := range proxyReports {
		err := validationutil.GetProxyError(rpt)
		if err != nil {
			proxyErrs = append(proxyErrs, err)
		}
	}
	if len(proxyErrs) > 0 {
		validationErr = errors.Errorf("resource incompatible with current Gloo snapshot: %v", proxyErrs)
	}

	details := &metav1.StatusDetails{
		Name:   req.Name,
		Group:  gvk.Group,
		Kind:   gvk.Kind,
		Causes: wh.getFailureCauses(proxyReports),
	}

	return &AdmissionResponseWithProxies{
		AdmissionResponse: &v1beta1.AdmissionResponse{
			Result: &metav1.Status{
				Message: validationErr.Error(),
				Details: details,
			},
		},
		Proxies: proxies,
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

func (wh *gatewayValidationWebhook) validate(ctx context.Context, gvk schema.GroupVersionKind, ref core.ResourceRef, rawJson []byte, isDelete, dryRun bool) (validation.ProxyReports, error) {

	switch gvk {
	case ListGVK:
		return wh.validateList(ctx, rawJson, dryRun)
	case gwv1.GatewayGVK:
		if isDelete {
			// we don't validate gateway deletion
			break
		}
		return wh.validateGateway(ctx, rawJson, dryRun)
	case gwv1.VirtualServiceGVK:
		if isDelete {
			return validation.ProxyReports{}, wh.validator.ValidateDeleteVirtualService(ctx, ref, dryRun)
		} else {
			return wh.validateVirtualService(ctx, rawJson, dryRun)
		}
	case gwv1.RouteTableGVK:
		if isDelete {
			return validation.ProxyReports{}, wh.validator.ValidateDeleteRouteTable(ctx, ref, dryRun)
		} else {
			return wh.validateRouteTable(ctx, rawJson, dryRun)
		}
	}
	return validation.ProxyReports{}, nil

}

func (wh *gatewayValidationWebhook) validateList(ctx context.Context, rawJson []byte, dryRun bool) (validation.ProxyReports, error) {
	var (
		ul           unstructured.UnstructuredList
		proxyReports validation.ProxyReports
		err          error
	)

	if err := ul.UnmarshalJSON(rawJson); err != nil {
		return nil, WrappedUnmarshalErr(err)
	}
	if proxyReports, err = wh.validator.ValidateList(ctx, &ul, dryRun); err != nil {
		return proxyReports, errors.Wrapf(err, "Validating %T failed", ul)
	}
	return proxyReports, nil
}

func (wh *gatewayValidationWebhook) validateGateway(ctx context.Context, rawJson []byte, dryRun bool) (validation.ProxyReports, error) {
	var (
		gw           gwv1.Gateway
		proxyReports validation.ProxyReports
		err          error
	)
	if err := protoutils.UnmarshalResource(rawJson, &gw); err != nil {
		return nil, WrappedUnmarshalErr(err)
	}
	if skipValidationCheck(gw.Metadata.Annotations) {
		return nil, nil
	}
	if proxyReports, err = wh.validator.ValidateGateway(ctx, &gw, dryRun); err != nil {
		return proxyReports, errors.Wrapf(err, "Validating %T failed", gw)
	}
	return proxyReports, nil
}

func (wh *gatewayValidationWebhook) validateVirtualService(ctx context.Context, rawJson []byte, dryRun bool) (validation.ProxyReports, error) {
	var (
		vs           gwv1.VirtualService
		proxyReports validation.ProxyReports
		err          error
	)
	if err := protoutils.UnmarshalResource(rawJson, &vs); err != nil {
		return nil, WrappedUnmarshalErr(err)
	}
	if skipValidationCheck(vs.Metadata.Annotations) {
		return nil, nil
	}
	if proxyReports, err = wh.validator.ValidateVirtualService(ctx, &vs, dryRun); err != nil {
		return proxyReports, errors.Wrapf(err, "Validating %T failed", vs)
	}
	return proxyReports, nil
}

func (wh *gatewayValidationWebhook) validateRouteTable(ctx context.Context, rawJson []byte, dryRun bool) (validation.ProxyReports, error) {
	var (
		rt           gwv1.RouteTable
		proxyReports validation.ProxyReports
		err          error
	)
	if err := protoutils.UnmarshalResource(rawJson, &rt); err != nil {
		return nil, WrappedUnmarshalErr(err)
	}
	if skipValidationCheck(rt.Metadata.Annotations) {
		return nil, nil
	}
	if proxyReports, err = wh.validator.ValidateRouteTable(ctx, &rt, dryRun); err != nil {
		return proxyReports, errors.Wrapf(err, "Validating %T failed", rt)
	}
	return proxyReports, nil
}
