package translator

type Opts struct {
	WriteNamespace                 string
	Validation                     *ValidationOpts
	IsolateVirtualHostsBySslConfig bool
	ReadGatewaysFromAllNamespaces  bool
}

type ValidationOpts struct {
	ProxyValidationServerAddress string
	ValidatingWebhookPort        int
	ValidatingWebhookCertPath    string
	ValidatingWebhookKeyPath     string
	IgnoreProxyValidationFailure bool
	AlwaysAcceptResources        bool
	AllowWarnings                bool
	WarnOnRouteShortCircuiting   bool
}
