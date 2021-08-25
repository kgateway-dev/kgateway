package k8sadmission

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"k8s.io/api/admission/v1beta1"

	"github.com/solo-io/go-utils/contextutils"
)

// GlooValidationHandler handles HTTP requests to validate gloo resources.
type GlooValidationHandler struct {
	ctx context.Context
}

func NewGlooValidationHandler(ctx context.Context) http.Handler {
	return &GlooValidationHandler{
		ctx: ctx,
	}
}

func (handler *GlooValidationHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	contextutils.LoggerFrom(handler.ctx).Infow("Received validation request")
	// TODO: do some actual validation
	admissionResponse := makeAllowedAdmissionResponse()
	admissionReview := makeAdmissionReview(admissionResponse)
	handler.writeResponse(&w, admissionReview)
}

// writeResponse marshals the AdmissionReview to JSON, then writes it using the provided ResponseWriter.
func (handler *GlooValidationHandler) writeResponse(w *http.ResponseWriter, review *v1beta1.AdmissionReview) {
	logger := contextutils.LoggerFrom(handler.ctx)
	writer := *w

	resp, err := json.Marshal(review)
	if err != nil {
		logger.Errorf("Can't encode response: %v", err)
		http.Error(writer, fmt.Sprintf("could not encode response: %v", err), http.StatusInternalServerError)
		return
	}
	logger.Infof("Ready to write response ...")
	if _, err := writer.Write(resp); err != nil {
		logger.Errorf("Can't write response: %v", err)
		http.Error(writer, fmt.Sprintf("could not write response: %v", err), http.StatusInternalServerError)
	}
	logger.Debugf("responded with review: %s", resp)
}

func makeAllowedAdmissionResponse() *v1beta1.AdmissionResponse {
	return makeAdmissionResponse(true)
}

func makeAdmissionResponse(allowed bool) *v1beta1.AdmissionResponse {
	return &v1beta1.AdmissionResponse{
		Allowed: allowed,
	}
}

func makeAdmissionReview(response *v1beta1.AdmissionResponse) *v1beta1.AdmissionReview {
	return &v1beta1.AdmissionReview{
		Response: response,
	}
}
