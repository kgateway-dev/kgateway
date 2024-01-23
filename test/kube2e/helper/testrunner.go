package helper

import (
	"fmt"
	"time"

	"github.com/solo-io/go-utils/log"
)

const (
	defaultTestRunnerImage = "quay.io/solo-io/testrunner:v1.7.0-beta17"
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

func NewTestRunner(namespace string) (*testRunner, error) {
	testContainer, err := newTestContainer(namespace, defaultTestRunnerImage, TestrunnerName, TestRunnerPort)
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

func (t *testRunner) Deploy(timeout time.Duration) error {
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
