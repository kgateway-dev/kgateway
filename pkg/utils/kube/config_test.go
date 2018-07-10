package kube

import (
	"io/ioutil"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/solo-io/gloo/test/helpers"
)

var _ = Describe("KubeConfigUtils", func() {
	Describe("load kubeconfig", func() {
		defaultHost := "https://1.2.3.4"
		var (
			configWithAll         string
			configWithOnlyContext string
		)
		BeforeEach(func() {
			configStr1 := `apiVersion: v1
clusters:
- cluster:
    insecure-skip-tls-verify: true
    server: ` + defaultHost + `
  name: development
contexts:
- context:
    cluster: development
    namespace: frontend
    user: developer
  name: dev-frontend
current-context: dev-frontend
kind: Config
preferences: {}
users:
- name: developer
  user:
    password: some-password
    username: exp`
			var err error
			configWithAll, err = generateKubeConfig(configStr1)
			Must(err)

			configStr2 := `apiVersion: v1
kind: Config
preferences: {}
contexts:
- context:
    cluster: development
    namespace: ramp
    user: developer
  name: dev-ramp-up`
			configWithOnlyContext, err = generateKubeConfig(configStr2)
			Must(err)
		})
		AfterEach(func() {
			os.RemoveAll(filepath.Dir(configWithAll))
		})
		Context("when a minimum kube config is set", func() {
			It("should load the kube config", func() {
				config, err := GetConfig("", configWithAll)
				Must(err)
				Expect(config.Host).To(Equal(defaultHost))
			})
		})
		Context("when master url is set and the kube config does not contain a host url", func() {
			It("should set the master url as the host", func() {
				host := "https://9.9.9.9"
				config, err := GetConfig(host, configWithOnlyContext)
				Must(err)
				Expect(config.Host).To(Equal(host))
			})
		})
		Describe("from env var KUBECONFIG", func() {
			var currentEnv string
			BeforeEach(func() {
				currentEnv = os.Getenv("KUBECONFIG")
			})
			AfterEach(func() {
				os.Setenv("KUBECONFIG", currentEnv)
			})
			Context("when KUBECONFIG is set with a single path", func() {
				It("should load the specified kube config", func() {
					os.Setenv("KUBECONFIG", configWithAll)
					config, err := GetConfig("", "")
					Must(err)
					Expect(config.Host).To(Equal(defaultHost))
				})
			})
			Context("when KUBECONFIG is set with multiple paths (first one containing the host url)", func() {
				It("should load the merged kube config without errors", func() {
					os.Setenv("KUBECONFIG", configWithAll+":"+configWithOnlyContext)
					config, err := GetConfig("", "")
					Must(err)
					Expect(config.Host).To(Equal(defaultHost))
				})
			})
			Context("when KUBECONFIG is set with multiple paths (second one containing the host url)", func() {
				It("should load the merged kube config without errors", func() {
					os.Setenv("KUBECONFIG", configWithOnlyContext+":"+configWithAll)
					config, err := GetConfig("", "")
					Must(err)
					Expect(config.Host).To(Equal(defaultHost))
				})
			})
			Context("when KUBECONFIG is set with multiple paths (first one with invalid path)", func() {
				It("should load the kube config without errors", func() {
					os.Setenv("KUBECONFIG", "invalid:"+configWithAll)
					config, err := GetConfig("", "")
					Must(err)
					Expect(config.Host).To(Equal(defaultHost))
				})
			})
			Context("when KUBECONFIG is set with multiple paths (second one with invalid path)", func() {
				It("should load the kube config without errors", func() {
					os.Setenv("KUBECONFIG", configWithAll+":invalid")
					config, err := GetConfig("", "")
					Must(err)
					Expect(config.Host).To(Equal(defaultHost))
				})
			})
		})
	})
})

func generateKubeConfig(configString string) (string, error) {
	tempDir, err := ioutil.TempDir("/tmp/", ".kube")
	if err != nil {
		return "", err
	}
	path := filepath.Join(tempDir, "config")

	err = ioutil.WriteFile(path, []byte(configString), 0644)
	if err != nil {
		return "", err
	}
	return path, nil
}
