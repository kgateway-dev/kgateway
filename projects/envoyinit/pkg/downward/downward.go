package downward

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// read downward api:

func CreateLocationReader(basedir string) func(string) ([]byte, error) {
	return func(f string) ([]byte, error) {
		fpath := filepath.Join(basedir, f)
		return os.ReadFile(fpath)
	}
}

func RetrieveDownwardAPI() DownwardAPI {
	return RetrieveDownwardAPIFrom(CreateLocationReader("/etc/podinfo/"), os.Getenv)
}

func TestNeededDownwardAPI() *TestWhichIsNeedDownwardAPI {
	return &TestWhichIsNeedDownwardAPI{}
}

type TestWhichIsNeedDownwardAPI struct {
	IsPodName       bool
	IsPodNamespace  bool
	IsPodIp         bool
	IsPodSvcAccount bool
	IsPodUID        bool

	IsNodeName bool
	IsNodeIp   bool

	IsPodLabels      bool
	IsPodAnnotations bool
}

func (td *TestWhichIsNeedDownwardAPI) PodName() string {
	td.IsPodName = true
	return ""
}

func (td *TestWhichIsNeedDownwardAPI) PodNamespace() string {
	td.IsPodNamespace = true
	return ""
}

func (td *TestWhichIsNeedDownwardAPI) PodIp() string {
	td.IsPodIp = true
	return ""
}

func (td *TestWhichIsNeedDownwardAPI) PodSvcAccount() string {
	td.IsPodSvcAccount = true
	return ""
}

func (td *TestWhichIsNeedDownwardAPI) PodUID() string {
	td.IsPodUID = true
	return ""
}

func (td *TestWhichIsNeedDownwardAPI) NodeName() string {
	td.IsNodeName = true
	return ""
}

func (td *TestWhichIsNeedDownwardAPI) NodeIp() string {
	td.IsNodeIp = true
	return ""
}

func (td *TestWhichIsNeedDownwardAPI) PodLabels() map[string]string {
	td.IsPodLabels = true
	return map[string]string{}
}

func (td *TestWhichIsNeedDownwardAPI) PodAnnotations() map[string]string {
	td.IsPodAnnotations = true
	return map[string]string{}
}

func RetrieveDownwardAPIFrom(read func(string) ([]byte, error), getenv func(string) string) DownwardAPI {
	// read annotations
	var ret downwardInjectable
	if labels, err := read("labels"); err == nil {
		ret.podLabels = parse(labels)
	}
	if annotations, err := read("annotations"); err == nil {
		ret.podAnnotations = parse(annotations)
	}

	ret.podIp = getenv("POD_IP")
	ret.podName = getenv("POD_NAME")
	ret.podNamespace = getenv("POD_NAMESPACE")

	ret.nodeName = getenv("NODE_NAME")
	ret.nodeIp = getenv("NODE_IP")

	ret.podUID = getenv("POD_UID")
	ret.podSvcAccount = getenv("POD_SVCACCNT")

	return &ret
}

type downwardInjectable struct {
	podName        string
	podNamespace   string
	podIp          string
	podSvcAccount  string
	podUID         string
	nodeName       string
	nodeIp         string
	podLabels      map[string]string
	podAnnotations map[string]string
}

func (di *downwardInjectable) PodName() string                   { return di.podName }
func (di *downwardInjectable) PodNamespace() string              { return di.podNamespace }
func (di *downwardInjectable) PodIp() string                     { return di.podIp }
func (di *downwardInjectable) PodSvcAccount() string             { return di.podSvcAccount }
func (di *downwardInjectable) PodUID() string                    { return di.podUID }
func (di *downwardInjectable) NodeName() string                  { return di.nodeName }
func (di *downwardInjectable) NodeIp() string                    { return di.nodeIp }
func (di *downwardInjectable) PodLabels() map[string]string      { return di.podLabels }
func (di *downwardInjectable) PodAnnotations() map[string]string { return di.podAnnotations }

func parse(data []byte) map[string]string {
	m := map[string]string{}

	lines := strings.Split(string(data), "\n")
	for _, l := range lines {
		l = strings.TrimSpace(l)
		parts := strings.SplitN(l, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := parts[0]
		value, err := strconv.Unquote(parts[1])
		if err != nil {
			continue
		}
		m[key] = value
	}
	return m
}
