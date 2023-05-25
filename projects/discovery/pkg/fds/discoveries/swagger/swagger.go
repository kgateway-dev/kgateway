package swagger

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/go-openapi/loads"
	openapi "github.com/go-openapi/spec"
	"github.com/go-openapi/swag"
	"github.com/hashicorp/go-multierror"
	errors "github.com/rotisserie/eris"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/log"

	"github.com/solo-io/gloo/projects/discovery/pkg/fds"
	transformation_plugins "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/transformation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/graphql/v1beta1"
	plugins "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options"
	rest_plugins "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/rest"
)

var commonSwaggerURIs = []string{
	"/swagger.json",
	"/swagger/docs/v1",
	"/swagger/docs/v2",
	"/v1/swagger",
	"/v2/swagger",
}

// TODO(yuval-k): run this in a back off for a limited amount of time, with high initial retry.
// maybe backoff with initial 1 minute a total of 10 minutes till giving up. this should probably be configurable

func NewFunctionDiscoveryFactory() fds.FunctionDiscoveryFactory {
	return &SwaggerFunctionDiscoveryFactory{
		DetectionTimeout: time.Minute,
		FunctionPollTime: time.Second * 15,
	}
}

type SwaggerFunctionDiscoveryFactory struct {
	DetectionTimeout time.Duration
	FunctionPollTime time.Duration
	SwaggerUrisToTry []string
	GraphqlClient    v1beta1.GraphQLApiClient
}

func (f *SwaggerFunctionDiscoveryFactory) NewFunctionDiscovery(u *v1.Upstream, _ fds.AdditionalClients) fds.UpstreamFunctionDiscovery {
	return &SwaggerFunctionDiscovery{
		detectionTimeout: f.DetectionTimeout,
		functionPollTime: f.FunctionPollTime,
		swaggerUrisToTry: append(f.SwaggerUrisToTry, commonSwaggerURIs...),
		upstream:         u,
	}
}

type SwaggerFunctionDiscovery struct {
	detectionTimeout time.Duration
	functionPollTime time.Duration
	upstream         *v1.Upstream
	swaggerUrisToTry []string
}

func getSwagSpec(u *v1.Upstream) *rest_plugins.ServiceSpec_SwaggerInfo {
	spec, ok := u.GetUpstreamType().(v1.ServiceSpecGetter)
	if !ok {
		return nil
	}
	serviceSpec := spec.GetServiceSpec()
	if serviceSpec == nil {
		return nil
	}
	restWrapper, ok := serviceSpec.GetPluginType().(*plugins.ServiceSpec_Rest)
	if !ok {
		return nil
	}
	rest := restWrapper.Rest
	return rest.GetSwaggerInfo()
}

func (f *SwaggerFunctionDiscovery) IsFunctional() bool {
	return getSwagSpec(f.upstream) != nil
}

func (f *SwaggerFunctionDiscovery) DetectType(ctx context.Context, baseurl *url.URL) (*plugins.ServiceSpec, error) {
	return f.detectUpstreamTypeOnce(ctx, baseurl)
}

func (f *SwaggerFunctionDiscovery) detectUpstreamTypeOnce(ctx context.Context, baseUrl *url.URL) (*plugins.ServiceSpec, error) {
	// run detection and get functions
	var errs error
	logger := contextutils.LoggerFrom(ctx)

	logger.Debugf("attempting to detect swagger base url %v", baseUrl)

	switch baseUrl.Scheme {
	case "http":
		fallthrough
	case "https":
		// nothing to do as this baseurl already has an http address.
	case "tcp":
		// if it is a tcp address, assume it is plain http
		baseUrl.Scheme = "http"
	default:
		return nil, fmt.Errorf("unsupported baseurl for swagger discovery %v", baseUrl)
	}

	for _, uri := range f.swaggerUrisToTry {
		url := baseUrl.ResolveReference(&url.URL{Path: uri}).String()
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, errors.Wrap(err, "invalid url for request")
		}
		req.Header.Set("X-Gloo-Discovery", "Swagger-Discovery")

		req = req.WithContext(ctx)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, multierror.Append(err, ctx.Err())
			}
			errs = multierror.Append(errs, errors.Wrapf(err, "could not perform HTTP GET on resolved addr: %v", url))
			continue
		}
		// might have found a swagger service
		if res.StatusCode == http.StatusOK {
			if _, err := RetrieveSwaggerDocFromUrl(ctx, url); err != nil {
				// first check if this is a context error
				if ctx.Err() != nil {
					return nil, multierror.Append(err, ctx.Err())
				}
				errs = multierror.Append(errs, err)
				continue
			}
			// definitely found swagger
			logger.Infof("swagger upstream detected: %v", url)
			svcInfo := &plugins.ServiceSpec{
				PluginType: &plugins.ServiceSpec_Rest{
					Rest: &rest_plugins.ServiceSpec{
						SwaggerInfo: &rest_plugins.ServiceSpec_SwaggerInfo{
							SwaggerSpec: &rest_plugins.ServiceSpec_SwaggerInfo_Url{
								Url: url,
							},
						},
					},
				},
			}
			return svcInfo, nil
		}
		errs = multierror.Append(errs, errors.Errorf("path: %v response code: %v headers: %v", uri, res.Status, res.Header))

	}
	logger.Debugf("failed to detect swagger for %s: %v", baseUrl.String(), errs.Error())
	// not a swagger upstream
	return nil, errors.Wrapf(errs, "service at %s does not implement swagger at a known endpoint, "+
		"or was unreachable", baseUrl.String())

}

func (f *SwaggerFunctionDiscovery) DetectFunctions(ctx context.Context, _ *url.URL, _ func() fds.Dependencies, updatecb func(fds.UpstreamMutator) error) error {
	in := f.upstream
	spec := getSwagSpec(in)
	if spec == nil || spec.GetSwaggerSpec() == nil {
		// TODO: make this a fatal error that avoids restarts?
		return errors.New("upstream doesn't have a swagger spec")
	}
	switch document := spec.GetSwaggerSpec().(type) {
	case *rest_plugins.ServiceSpec_SwaggerInfo_Url:
		return f.detectFunctionsFromUrl(ctx, document.Url, in, updatecb)
	case *rest_plugins.ServiceSpec_SwaggerInfo_Inline:
		return f.detectFunctionsFromInline(ctx, document.Inline, in, updatecb)
	}

	return errors.New("upstream doesn't have a swagger source")
}

func (f *SwaggerFunctionDiscovery) detectFunctionsFromUrl(ctx context.Context, url string, in *v1.Upstream, updatecb func(fds.UpstreamMutator) error) error {
	err := contextutils.NewExponentialBackoff(contextutils.ExponentialBackoff{}).Backoff(ctx, func(ctx context.Context) error {

		spec, err := RetrieveSwaggerDocFromUrl(ctx, url)
		if err != nil {
			return err
		}
		err = f.detectFunctionsFromSpec(ctx, spec, in, updatecb)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		contextutils.LoggerFrom(ctx).Warnf("Unable to perform Swagger function discovery for upstream %s in namespace %s, error: ",
			f.upstream.GetMetadata().GetName(),
			f.upstream.GetMetadata().GetNamespace(),
			err.Error(),
		)
		// ignore other errors as we would like to continue forever.
	}
	if err := contextutils.Sleep(ctx, f.functionPollTime); err != nil {
		return err
	}
	return nil
}

func (f *SwaggerFunctionDiscovery) detectFunctionsFromInline(ctx context.Context, document string, in *v1.Upstream, updatecb func(fds.UpstreamMutator) error) error {
	spec, err := parseSwaggerDoc([]byte(document))
	if err != nil {
		return err
	}
	return f.detectFunctionsFromSpec(ctx, spec, in, updatecb)
}

func (f *SwaggerFunctionDiscovery) detectFunctionsFromSpec(ctx context.Context, swaggerSpec *openapi.Swagger, in *v1.Upstream, updatecb func(fds.UpstreamMutator) error) error {
	var consumesJson bool
	if len(swaggerSpec.Consumes) == 0 {
		consumesJson = true
	}
	for _, contentType := range swaggerSpec.Consumes {
		if contentType == "application/json" {
			consumesJson = true
			break
		}
	}
	if !consumesJson {
		return errors.Errorf("swagger function discovery uses content type application/json; "+
			"available: %v", swaggerSpec.Consumes)
	}
	// TODO: when response transformation is done, look at produces as well

	funcs := make(map[string]*transformation_plugins.TransformationTemplate)

	if swaggerSpec.Paths == nil {
		return errors.Errorf("swagger spec paths was nil: %v", swaggerSpec.Paths)
	}

	for functionPath, pathItem := range swaggerSpec.Paths.Paths {
		createFunctionsForPath(funcs, swaggerSpec.BasePath, functionPath, pathItem.PathItemProps, swaggerSpec.Definitions)
	}

	return updatecb(func(u *v1.Upstream) error {
		upstreamSpec, ok := u.GetUpstreamType().(v1.ServiceSpecMutator)
		if !ok {
			return errors.New("not a valid upstream")
		}
		spec := upstreamSpec.GetServiceSpec()
		if spec == nil {
			spec = &plugins.ServiceSpec{}
		}
		restSpec, ok := spec.GetPluginType().(*plugins.ServiceSpec_Rest)
		if !ok {
			restSpec = &plugins.ServiceSpec_Rest{
				Rest: &rest_plugins.ServiceSpec{},
			}
		}

		restSpec.Rest.Transformations = funcs
		spec.PluginType = restSpec

		upstreamSpec.SetServiceSpec(spec)
		return nil
	})
}

func RetrieveSwaggerDocFromUrl(ctx context.Context, url string) (*openapi.Swagger, error) {
	docBytes, err := LoadFromFileOrHTTP(ctx, url)
	if err != nil {
		return nil, errors.Wrap(err, "loading swagger doc from url")
	}
	return parseSwaggerDoc(docBytes)
}

func LoadFromFileOrHTTP(ctx context.Context, url string) ([]byte, error) {
	return swag.LoadStrategy(url, os.ReadFile, loadHTTPBytes(ctx))(url)
}

func loadHTTPBytes(ctx context.Context) func(path string) ([]byte, error) {
	return func(path string) ([]byte, error) {
		req, err := http.NewRequest("GET", path, nil)
		if err != nil {
			return nil, err
		}
		req = req.WithContext(ctx)
		resp, err := http.DefaultClient.Do(req)
		defer func() {
			if resp != nil {
				if e := resp.Body.Close(); e != nil {
					contextutils.LoggerFrom(ctx).Debug(e)
				}
			}
		}()
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("could not access document at %q [%s] ", path, resp.Status)
		}

		return io.ReadAll(resp.Body)
	}
}

func parseSwaggerDoc(docBytes []byte) (*openapi.Swagger, error) {
	doc, err := loads.Analyzed(docBytes, "")
	if err != nil {
		log.Debugf("parsing doc as json failed, falling back to yaml")
		jsn, err := swag.YAMLToJSON(docBytes)
		if err != nil {
			return nil, errors.Wrap(err, "failed to convert yaml to json (after falling back to yaml parsing)")
		}
		doc, err = loads.Analyzed(jsn, "")
		if err != nil {
			return nil, errors.Wrap(err, "invalid swagger doc")
		}
	}
	return doc.Spec(), nil
}
