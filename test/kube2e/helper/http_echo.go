package helper

const (
	defaultHttpEchoImage = "kennship/http-echo@sha256:144322e8e96be2be6675dcf6e3ee15697c5d052d14d240e8914871a2a83990af"
	HttpEchoName         = "http-echo"
	HttpEchoPort         = 3000
)

func NewEchoHttp(namespace string) (TestContainer, error) {
	return newTestContainer(namespace, defaultHttpEchoImage, HttpEchoName, HttpEchoPort)
}

const (
	defaultTcpEchoImage = "soloio/tcp-echo:latest"
	TcpEchoName         = "tcp-echo"
	TcpEchoPort         = 1025
)

func NewEchoTcp(namespace string) (TestContainer, error) {
	return newTestContainer(namespace, defaultTcpEchoImage, TcpEchoName, TcpEchoPort)
}
