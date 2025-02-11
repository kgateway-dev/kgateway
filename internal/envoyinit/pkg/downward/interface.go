package downward

type DownwardAPI interface {
	PodName() string
	PodNamespace() string
	PodIp() string
	PodSvcAccount() string
	PodUID() string

	NodeName() string
	NodeIp() string

	PodLabels() map[string]string
	PodAnnotations() map[string]string
}
