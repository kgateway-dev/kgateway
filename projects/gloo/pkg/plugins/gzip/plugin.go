package gzip

import (
	envoycompressor "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/compressor/v3"
	envoygzip "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/gzip/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
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

	envoyGzipConfig, err := glooToEnvoyGzip(gzipConfig)
	if err != nil {
		return nil, eris.Wrapf(err, "converting gzip config")
	}
	gzipFilter, err := plugins.NewStagedFilterWithConfig(wellknown.Gzip, envoyGzipConfig, pluginStage)
	if err != nil {
		return nil, eris.Wrapf(err, "generating filter config")
	}

	return []plugins.StagedHttpFilter{gzipFilter}, nil
}

func glooToEnvoyGzip(gzip *v2.Gzip) (*envoygzip.Gzip, error) {

	envoyGzip := &envoygzip.Gzip{}

	if gzip.GetMemoryLevel() != nil {
		envoyGzip.MemoryLevel = &wrappers.UInt32Value{Value: gzip.GetMemoryLevel().GetValue()}
	}

	switch gzip.GetCompressionLevel() {
	case v2.Gzip_CompressionLevel_DEFAULT:
		envoyGzip.CompressionLevel = envoygzip.Gzip_CompressionLevel_DEFAULT
	case v2.Gzip_CompressionLevel_BEST:
		envoyGzip.CompressionLevel = envoygzip.Gzip_CompressionLevel_BEST
	case v2.Gzip_CompressionLevel_SPEED:
		envoyGzip.CompressionLevel = envoygzip.Gzip_CompressionLevel_SPEED
	default:
		return &envoygzip.Gzip{}, eris.Errorf("invalid CompressionLevel %v", gzip.GetCompressionLevel())
	}

	switch gzip.GetCompressionStrategy() {
	case v2.Gzip_DEFAULT:
		envoyGzip.CompressionStrategy = envoygzip.Gzip_DEFAULT
	case v2.Gzip_FILTERED:
		envoyGzip.CompressionStrategy = envoygzip.Gzip_FILTERED
	case v2.Gzip_HUFFMAN:
		envoyGzip.CompressionStrategy = envoygzip.Gzip_HUFFMAN
	case v2.Gzip_RLE:
		envoyGzip.CompressionStrategy = envoygzip.Gzip_RLE
	default:
		return &envoygzip.Gzip{}, eris.Errorf("invalid CompressionStrategy %v", gzip.GetCompressionStrategy())
	}

	if gzip.GetWindowBits() != nil {
		envoyGzip.WindowBits = &wrappers.UInt32Value{Value: gzip.GetWindowBits().GetValue()}
	}

	contentLength := gzip.GetContentLength()
	contentType := gzip.GetContentType()
	disableOnEtagHeader := gzip.GetDisableOnEtagHeader()
	removeAcceptEncodingHeader := gzip.GetRemoveAcceptEncodingHeader()

	// Envoy API has changed. v2.Gzip is based on an old Envoy API with several now deprecated fields.
	containsOldFields := contentLength != nil || contentType != nil || disableOnEtagHeader || removeAcceptEncodingHeader

	// Include the data from deprecated fields in the new Compressor field.
	if containsOldFields {
		envoyGzip.Compressor = &envoycompressor.Compressor{
			ContentType:                contentType,
			DisableOnEtagHeader:        disableOnEtagHeader,
			RemoveAcceptEncodingHeader: removeAcceptEncodingHeader,
		}
		if contentLength != nil {
			envoyGzip.Compressor.ContentLength = &wrappers.UInt32Value{Value: contentLength.GetValue()}
		}
	}

	// ChunkSize field isn't used in v2.Gzip, so it should always be nil

	return envoyGzip, nil
}
