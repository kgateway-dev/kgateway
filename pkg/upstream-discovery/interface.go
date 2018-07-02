package upstreamdiscovery

type Controller interface {
	Run(stop <-chan struct{})
	Error() <-chan error
}
