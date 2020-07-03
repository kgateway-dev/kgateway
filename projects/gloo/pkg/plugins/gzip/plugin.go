package gzip

import (
	envoycompressor "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/compressor/v3"
	envoygzip "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/gzip/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/gogo/protobuf/jsonpb"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/rotisserie/eris"
	v2 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/filter/http/gzip/v2"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

// filter should be called after routing decision has been made
var pluginStage = plugins.DuringStage(plugins.RouteStage)

func NewPlugin() *Plugin {
	return &Plugin{}
}

var _ plugins.Plugin = new(Plugin)
var _ plugins.HttpFilterPlugin = new(Plugin)

type Plugin struct {
}

func (p *Plugin) Init(params plugins.InitParams) error {
	return nil
}

func (p *Plugin) HttpFilters(_ plugins.Params, listener *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {

	gzipConfig := listener.GetOptions().GetGzip()

	if gzipConfig == nil {
		return nil, nil
	}

	envoyGzipConfig, err := convertGzip(gzipConfig) // Note: this modifies the gzipConfig
	if err != nil {
		return nil, eris.Wrapf(err, "converting filter")
	}
	gzipFilter, err := plugins.NewStagedFilterWithConfig(wellknown.Gzip, envoyGzipConfig, pluginStage)
	if err != nil {
		return nil, eris.Wrapf(err, "generating filter config")
	}

	return []plugins.StagedHttpFilter{gzipFilter}, nil
}

func convertGzip(gzip *v2.Gzip) (*envoygzip.Gzip, error) {

	contentLength := gzip.GetContentLength()
	contentType := gzip.GetContentType()
	disableOnEtagHeader := gzip.GetDisableOnEtagHeader()
	removeAcceptEncodingHeader := gzip.GetRemoveAcceptEncodingHeader()

	// Envoy API has changed. v2.Gzip is based on an old Envoy API with several now deprecated fields.
	containsOldFields := contentLength != nil || contentType != nil || disableOnEtagHeader || removeAcceptEncodingHeader

	if containsOldFields {
		// Adjust `gzip` so we can convert it using json as an intermediary.
		gzip.ContentLength = nil
		gzip.ContentType = nil
		gzip.DisableOnEtagHeader = false
		gzip.RemoveAcceptEncodingHeader = false
	}

	// convert type from v2.Gzip to envoygzip.Gzip using json as an intermediary
	jase := jsonpb.Marshaler{}
	gzipStr, err := jase.MarshalToString(gzip)
	if err != nil {
		return nil, err
	}
	remarshalled := new(envoygzip.Gzip)
	if err := jsonpb.UnmarshalString(gzipStr, remarshalled); err != nil {
		return nil, err
	}

	// Adjust `remarshalled` to include the data from deprecated fields in the new Compressor field.
	if containsOldFields {
		remarshalled.Compressor = &envoycompressor.Compressor{
			ContentType: contentType,
			DisableOnEtagHeader: disableOnEtagHeader,
			RemoveAcceptEncodingHeader: removeAcceptEncodingHeader,
		}
		if contentLength != nil {
			remarshalled.Compressor.ContentLength = &wrappers.UInt32Value{Value: contentLength.GetValue()}
		}
	}

	return remarshalled, nil
}
