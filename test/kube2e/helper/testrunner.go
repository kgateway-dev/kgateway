package helper

import (
	"fmt"
	"os"
	"time"

	"github.com/pkg/errors"
	"github.com/solo-io/go-utils/log"
)

const (
	defaultTestServerImage = "quay.io/solo-io/testrunner:v1.7.0-beta17"
	TestrunnerName         = "testrunner"
	TestRunnerPort         = 1234

	// This response is given by the testrunner when the SimpleServer is started
	SimpleHttpResponse = `<!DOCTYPE html PUBLIC "-//W3C//DTD HTML 3.2 Final//EN"><html>
<title>Directory listing for /</title>
<body>
<h2>Directory listing for /</h2>
<hr>
<ul>
<li><a href="bin/">bin/</a>
<li><a href="boot/">boot/</a>
<li><a href="dev/">dev/</a>
<li><a href="etc/">etc/</a>
<li><a href="home/">home/</a>
<li><a href="lib/">lib/</a>
<li><a href="lib64/">lib64/</a>
<li><a href="media/">media/</a>
<li><a href="mnt/">mnt/</a>
<li><a href="opt/">opt/</a>
<li><a href="proc/">proc/</a>
<li><a href="root/">root/</a>
<li><a href="root.crt">root.crt</a>
<li><a href="run/">run/</a>
<li><a href="sbin/">sbin/</a>
<li><a href="srv/">srv/</a>
<li><a href="sys/">sys/</a>
<li><a href="tmp/">tmp/</a>
<li><a href="usr/">usr/</a>
<li><a href="var/">var/</a>
</ul>
<hr>
</body>
</html>`
)

func NewTestRunner(namespace string) (TestUpstreamServer, error) {
	testContainer, err := newTestContainer(namespace, defaultTestServerImage, TestrunnerName, TestRunnerPort)
	if err != nil {
		return nil, err
	}

	return &testRunner{
		testContainer: testContainer,
	}, nil
}

// This object represents a container that gets deployed to the cluster to support testing.
type testRunner struct {
	*testContainer
}

func (t *testRunner) DeployServer(timeout time.Duration) error {
	err := t.deploy(timeout)
	if err != nil {
		return err
	}
	go func() {
		start := time.Now()
		log.Debugf("starting http server listening on port %v", TestRunnerPort)
		// This command start an http SimpleHttpServer and blocks until the server terminates
		if _, err := t.Exec("python", "-m", "SimpleHTTPServer", fmt.Sprintf("%v", TestRunnerPort)); err != nil {
			// if an error happened after 5 seconds, it's probably not an error.. just the pod terminating.
			if time.Now().Sub(start).Seconds() < 5.0 {
				log.Warnf("failed to start HTTP Server in Test Runner: %v", err)
			}
		}
	}()
	return nil
}

// DeployTLS deletes the running server pod and its service, then redeploys using a server
// which serves HTTPS with the cert and key provided.
func (t *testRunner) DeployServerTls(timeout time.Duration, crt, key []byte) error {
	if err := t.TerminatePodAndDeleteService(); err != nil {
		return errors.Wrap(err, "terminating pod and deleting service")
	}
	if err := t.testContainer.DeployResources(timeout); err != nil {
		return errors.Wrap(err, "deploying pod")
	}

	certFname := "/tmp/testrunner_tls/cert.pem"
	keyFname := "/tmp/testrunner_tls/key.pem"
	scriptFname := "/tmp/testrunner_tls/server.py"
	os.MkdirAll("/tmp/testrunner_tls", os.ModePerm)
	defer os.RemoveAll("/tmp/testrunner_tls")

	if err := os.WriteFile(certFname, crt, os.ModePerm); err != nil {
		return errors.Wrap(err, "writing cert")
	}
	if err := os.WriteFile(keyFname, key, os.ModePerm); err != nil {
		return errors.Wrap(err, "writing key")
	}
	if err := os.WriteFile(scriptFname, []byte(fmt.Sprintf(`
import BaseHTTPServer, SimpleHTTPServer
import ssl

httpd = BaseHTTPServer.HTTPServer(('0.0.0.0', %d), SimpleHTTPServer.SimpleHTTPRequestHandler)
httpd.socket = ssl.wrap_socket (httpd.socket, keyfile='/tmp/key.pem', certfile='/tmp/cert.pem', server_side=True)
httpd.serve_forever()
`, TestRunnerPort)), os.ModePerm); err != nil {
		return errors.Wrap(err, "writing server")
	}
	if err := t.Cp(map[string]string{
		certFname:   "/tmp/cert.pem",
		keyFname:    "/tmp/key.pem",
		scriptFname: "/tmp/server.py",
	}); err != nil {
		return errors.Wrap(err, "kubectl cp")
	}

	go func() {
		start := time.Now()
		log.Debugf("starting https server listening on port %v", TestRunnerPort)
		// This command starts an https SimpleHttpServer and blocks until the server terminates
		if _, err := t.Exec("python", "/tmp/server.py"); err != nil {
			// if an error happened after 5 seconds, it's probably not an error.. just the pod terminating.
			if time.Now().Sub(start).Seconds() < 5.0 {
				log.Warnf("failed to start HTTPS Server in Test Runner: %v", err)
			}
		}
	}()
	return nil
}
