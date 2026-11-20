package envoyinit

import (
	"errors"
	"io/fs"
	"log"
	"os"
	"time"
)

// inputConfigPollInterval is how often envoyinit checks for a missing bootstrap file
// that INPUT_CONF names.
const inputConfigPollInterval = time.Second

// waitForInputConfig waits until path exists when the INPUT_CONF environment variable
// names it. The controller sets INPUT_CONF to the ConfigMap key of the pod's own
// bootstrap revision. kubelet serves ConfigMap volumes from a watch-based cache, so a
// pod created right after the ConfigMap write can mount the previous content, without
// its revision's key, until kubelet syncs the volume. There is no time limit: the
// pod's startup probe bounds the wait. Without INPUT_CONF, envoyinit reads the default
// path and reports a missing file at once.
func waitForInputConfig(path string) {
	// Only a path the controller chose can be waited for.
	if os.Getenv(inputConfigPathEnv) == "" {
		return
	}

	waitForFile(path, inputConfigPollInterval, os.Stat, log.Printf)
}

// waitForFile returns once stat finds path, checking every interval while stat says it
// does not exist. Any other stat error returns at once, so reading the file reports it.
func waitForFile(path string, interval time.Duration, stat func(string) (fs.FileInfo, error), logf func(string, ...any)) {
	// Return at once when the file is there, or when stat fails for another reason.
	_, err := stat(path)
	if !errors.Is(err, fs.ErrNotExist) {
		return
	}

	// Poll until the file appears.
	logf("Waiting for the bootstrap file %s to appear in the mounted ConfigMap", path)
	start := time.Now()
	for errors.Is(err, fs.ErrNotExist) {
		time.Sleep(interval)
		_, err = stat(path)
	}
	logf("The bootstrap file %s appeared after %s", path, time.Since(start).Round(time.Millisecond))
}
