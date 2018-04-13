package azure

type azureFunc struct {
	Name               string         `json:"name"`
	FunctionAppID      interface{}    `json:"function_app_id"`
	ScriptRootPathHref string         `json:"script_root_path_href"`
	ScriptHref         string         `json:"script_href"`
	ConfigHref         string         `json:"config_href"`
	SecretsFileHref    string         `json:"secrets_file_href"`
	Href               string         `json:"href"`
	Config             functionConfig `json:"config"`
	Files              interface{}    `json:"files"`
	TestData           string         `json:"test_data"`
}

type functionConfig struct {
	Bindings functionConfigBindings `json:"bindings"`
	Disabled bool                   `json:"disabled"`
}

type functionConfigBindings []struct {
	AuthLevel string `json:"authLevel,omitempty"`
	Type      string `json:"type"`
	Direction string `json:"direction"`
	Name      string `json:"name"`
}

type masterKey struct {
	MasterKey string `json:"masterKey"`
}

type functionKey struct {
	Keys []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"keys"`
	Links []struct {
		Rel  string `json:"rel"`
		Href string `json:"href"`
	} `json:"links"`
}
