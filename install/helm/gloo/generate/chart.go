package generate

type Chart struct {
	ApiVersion  string   `json:"apiVersion"`
	Description string   `json:"description"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Home        string   `json:"home"`
	Icon        string   `json:"icon"`
	Sources     []string `json:"sources"`
}
