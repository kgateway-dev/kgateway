package helper

const (
	defaultHttpEchoImage = "kennship/http-echo@sha256:144322e8e96be2be6675dcf6e3ee15697c5d052d14d240e8914871a2a83990af"
	HttpEchoName         = "http-echo"
	HttpEchoPort         = 3000
)

// Deprecated
// ported to test/kubernetes/e2e/defaults/testdata/http_echo.yaml
func NewEchoHttp(namespace string) (TestContainer, error) {
	return newTestContainer(namespace, defaultHttpEchoImage, HttpEchoName, HttpEchoPort, true, nil)
}

const (
	defaultTcpEchoImage = "soloio/tcp-echo:latest"
	TcpEchoName         = "tcp-echo"
	TcpEchoPort         = 1025
)

// Deprecated
// ported to test/kubernetes/e2e/defaults/testdata/tcp_echo.yaml
func NewEchoTcp(namespace string) (TestContainer, error) {
	return newTestContainer(namespace, defaultTcpEchoImage, TcpEchoName, TcpEchoPort, true, nil)
}
