package envoyinit

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

func TestWaitForFilePresent(t *testing.T) {
	g := NewWithT(t)

	// Write the bootstrap file before waiting.
	path := filepath.Join(t.TempDir(), "envoy-abc.yaml")
	g.Expect(os.WriteFile(path, []byte("node: {}"), 0o600)).To(Succeed())

	// The wait returns without polling or logging.
	var logs []string
	waitForFile(path, time.Hour, os.Stat, func(format string, _ ...any) { logs = append(logs, format) })
	g.Expect(logs).To(BeEmpty(), "a present file should not be waited for")
}

func TestWaitForFileAppearsLater(t *testing.T) {
	g := NewWithT(t)

	// Report the file missing for the first three checks.
	path := "/etc/envoy/envoy-abc.yaml"
	calls := 0
	stat := func(name string) (fs.FileInfo, error) {
		calls++
		g.Expect(name).To(Equal(path))
		if calls <= 3 {
			return nil, fs.ErrNotExist
		}
		return nil, nil
	}

	// The wait returns once the file is found, logging the start and the end.
	var logs []string
	waitForFile(path, time.Millisecond, stat, func(format string, _ ...any) { logs = append(logs, format) })
	g.Expect(calls).To(Equal(4), "the wait should check until the file is found")
	g.Expect(logs).To(HaveLen(2), "the wait should log when it starts and when the file appears")
}

func TestWaitForFileOtherError(t *testing.T) {
	g := NewWithT(t)

	// Fail the check with an error other than a missing file.
	calls := 0
	stat := func(string) (fs.FileInfo, error) {
		calls++
		return nil, fs.ErrPermission
	}

	// The wait returns at once, leaving the read to report the error.
	waitForFile("/etc/envoy/envoy-abc.yaml", time.Hour, stat, func(string, ...any) {})
	g.Expect(calls).To(Equal(1), "only a missing file should be waited for")
}

func TestWaitForInputConfigWithoutEnv(t *testing.T) {
	g := NewWithT(t)

	// Leave INPUT_CONF unset and point at a file that never appears.
	t.Setenv(inputConfigPathEnv, "")
	path := filepath.Join(t.TempDir(), "envoy.yaml")

	// The wait returns at once.
	done := make(chan struct{})
	go func() {
		waitForInputConfig(path)
		close(done)
	}()
	g.Eventually(done).WithTimeout(5*time.Second).Should(BeClosed(), "without INPUT_CONF the default path should not be waited for")
}

func TestWaitForInputConfigWithEnv(t *testing.T) {
	g := NewWithT(t)

	// Set INPUT_CONF to a file that does not exist yet.
	path := filepath.Join(t.TempDir(), "envoy-abc.yaml")
	t.Setenv(inputConfigPathEnv, path)

	// Start waiting, and check the wait has not returned.
	done := make(chan struct{})
	go func() {
		waitForInputConfig(path)
		close(done)
	}()
	g.Consistently(done).WithTimeout(1500*time.Millisecond).ShouldNot(BeClosed(), "the wait should last while the file is missing")

	// Write the file; the wait returns.
	g.Expect(os.WriteFile(path, []byte("node: {}"), 0o600)).To(Succeed())
	g.Eventually(done).WithTimeout(5*time.Second).Should(BeClosed(), "the wait should return once the file appears")
}
