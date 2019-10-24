package license

type Product interface {
	// determines if a license should be skipped
	// sample reason for skipping: a library is only used on MacOS, not in Linux, which is the environment of the deployed process
	SkipLicense(license License) bool

	// This library generates license lists from binary dependencies but may exclude some external dependencies (such as EnvoyProxy)
	ExtraLicenses() []License

	// Replacements returns an even-length list of strings representing an old and new string
	ReplacementList() []string

	// If the automated tool is unable to correctly categorize a license you can override it with this function
	OverrideLicense(pkg, license string) string
}

var _ Product = &genericProduct{}

type genericProduct struct{}

func (gp *genericProduct) SkipLicense(l License) bool {
	return false
}

func (gp *genericProduct) ExtraLicenses() []License {
	return nil
}

func (gp *genericProduct) ReplacementList() []string {
	return CommonReplacements
}

func (gp *genericProduct) OverrideLicense(pkg, license string) string {
	return CommonOverrides(pkg, license)
}

var CommonReplacements = []string{
	"k8s.io", "github.com/kubernetes",
	"golang.org/x", "github.com/golang",
	"go.uber.org", "github.com/uber-go",
	"cloud.google.com/go", "github.com/googleapis/google-cloud-go",
	"google.golang.org/grpc", "github.com/grpc/grpc-go",
	"istio.io", "github.com/istio",
	"contrib.go.opencensus.io/exporter/prometheus", "github.com/census-ecosystem/opencensus-go-exporter-prometheus",
	"google.golang.org/genproto", "github.com/googleapis/go-genproto",
	"sigs.k8s.io", "github.com/kubernetes-sigs",
	"knative.dev", "github.com/knative",
}

const (
	MITForm       = "MIT License"
	Apache2_0Form = "Apache License 2.0"
)

func CommonOverrides(pkg, candidate string) string {
	switch pkg {
	case "github.com/ghodss/yaml":
		return MITForm
	case "github.com/jmespath/go-jmespath":
		return Apache2_0Form
	case "sigs.k8s.io/yaml":
		return MITForm
	default:
		return candidate
	}
}
