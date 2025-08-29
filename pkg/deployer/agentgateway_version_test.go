package deployer

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
)

func TestGetAgentgatewayVersionFromGoMod(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		name         string
		goModContent string
		expectedVer  string
		expectedErr  bool
	}{
		{
			name: "direct dependency with v prefix",
			goModContent: `module github.com/test/test

go 1.24.6

require (
	github.com/agentgateway/agentgateway v0.7.5
	github.com/other/dep v1.0.0
)`,
			expectedVer: "0.7.5",
			expectedErr: false,
		},
		{
			name: "direct dependency with different version",
			goModContent: `module github.com/test/test

go 1.24.6

require (
	github.com/agentgateway/agentgateway v0.8.0
	github.com/other/dep v1.0.0
)`,
			expectedVer: "0.8.0",
			expectedErr: false,
		},
		{
			name: "replaced version",
			goModContent: `module github.com/test/test

go 1.24.6

require (
	github.com/agentgateway/agentgateway v0.7.5
)

replace github.com/agentgateway/agentgateway => github.com/agentgateway/agentgateway v0.9.1`,
			expectedVer: "0.9.1",
			expectedErr: false,
		},
		{
			name: "replaced with local path",
			goModContent: `module github.com/test/test

go 1.24.6

require (
	github.com/agentgateway/agentgateway v0.7.5
)

replace github.com/agentgateway/agentgateway => ../local/agentgateway`,
			expectedVer: "",
			expectedErr: true,
		},
		{
			name: "multiple replace directives - last one is used",
			goModContent: `module github.com/test/test

go 1.24.6

require (
	github.com/agentgateway/agentgateway v0.7.5
)

replace github.com/agentgateway/agentgateway => github.com/agentgateway/agentgateway v0.8.0
replace github.com/agentgateway/agentgateway => github.com/agentgateway/agentgateway v0.9.2`,
			expectedVer: "0.9.2",
			expectedErr: false,
		},
		{
			name: "no agentgateway dependency",
			goModContent: `module github.com/test/test

go 1.24.6

require (
	github.com/other/dep v1.0.0
)`,
			expectedVer: "",
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "gomod-test-*")
			g.Expect(err).NotTo(HaveOccurred())
			defer os.RemoveAll(tmpDir)

			goModPath := filepath.Join(tmpDir, "go.mod")
			err = os.WriteFile(goModPath, []byte(tt.goModContent), 0644)
			g.Expect(err).NotTo(HaveOccurred())

			originalWd, err := os.Getwd()
			g.Expect(err).NotTo(HaveOccurred())
			err = os.Chdir(tmpDir)
			g.Expect(err).NotTo(HaveOccurred())
			defer os.Chdir(originalWd)

			version, err := getAgentgatewayVersionFromGoMod()
			if tt.expectedErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(version).To(Equal(tt.expectedVer))
			}
		})
	}
}

func TestNormalizeVersion(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		input    string
		expected string
	}{
		{"v1.2.3", "1.2.3"},
		{"1.2.3", "1.2.3"},
		{"v0.7.5", "0.7.5"},
		{"v1.0.0-alpha", "1.0.0-alpha"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeVersion(tt.input)
			g.Expect(result).To(Equal(tt.expected))
		})
	}
}

func TestGetAgentgatewayVersion(t *testing.T) {
	g := NewWithT(t)
	version := GetAgentgatewayVersion()
	g.Expect(version).NotTo(BeEmpty())
	g.Expect(version).To(Equal("0.7.8"))
}

func TestGetAgentgatewayVersionFallback(t *testing.T) {
	g := NewWithT(t)

	originalWd, err := os.Getwd()
	g.Expect(err).NotTo(HaveOccurred())

	tmpDir, err := os.MkdirTemp("", "nogomod-test-*")
	g.Expect(err).NotTo(HaveOccurred())
	defer os.RemoveAll(tmpDir)

	err = os.Chdir(tmpDir)
	g.Expect(err).NotTo(HaveOccurred())
	defer os.Chdir(originalWd)

	version := GetAgentgatewayVersion()
	g.Expect(version).To(Equal(AgentgatewayDefaultTag))
}
