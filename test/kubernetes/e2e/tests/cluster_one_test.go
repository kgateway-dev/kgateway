//go:build cluster_one

package tests_test

import (
	"testing"
)

// TestClusterOne is a placeholder test that is used to prevent the following error:
// package github.com/solo-io/gloo/test/kubernetes/e2e/tests: build constraints exclude all Go files in /home/runner/work/gloo/gloo/test/kubernetes/e2e/tests
// Until we migrate tests into this /tests folder, we need to ensure that there is _at least_ on test for each build tag
func TestClusterOne(t *testing.T) {
	// do nothing
	// This should be deleted as soon as we have one valid test run against cluster_one
}
