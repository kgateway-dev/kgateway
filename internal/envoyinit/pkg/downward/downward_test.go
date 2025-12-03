package downward_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "github.com/kgateway-dev/kgateway/v2/internal/envoyinit/pkg/downward"
)

var _ = Describe("Downward", func() {

	var data map[string][]byte
	var errors map[string]error
	var env map[string]string

	reader := func(what string) ([]byte, error) {
		if errors[what] != nil {
			return nil, errors[what]
		}
		return data[what], nil
	}
	envreader := func(what string) string { return env[what] }

	BeforeEach(func() {
		data = map[string][]byte{}
		errors = map[string]error{}
		env = map[string]string{}
	})

	It("should retrieve labels", func() {
		data["labels"] = []byte(`
		kgateway="kgateway.dev"

		`)

		res := RetrieveDownwardAPIFrom(reader, envreader)
		Expect(res.PodLabels()).To(HaveKeyWithValue("kgateway", "kgateway.dev"))
	})

	It("should ignore bad labels", func() {
		data["labels"] = []byte(`
		 bad label...
		not=quoted
		`)

		res := RetrieveDownwardAPIFrom(reader, envreader)
		Expect(res.PodLabels()).To(BeEmpty())
	})

	It("should retrieve annotations", func() {
		data["annotations"] = []byte(`
		kgateway="kgateway.dev"

		`)

		res := RetrieveDownwardAPIFrom(reader, envreader)
		Expect(res.PodAnnotations()).To(HaveKeyWithValue("kgateway", "kgateway.dev"))
	})

	It("should env vars", func() {
		env["POD_IP"] = "1.2.3.4"
		env["POD_NAME"] = "greatname"
		env["POD_NAMESPACE"] = "namespace"
		env["POD_UID"] = "uid"
		env["POD_SVCACCNT"] = "svcaccount"
		env["NODE_NAME"] = "nodename"
		env["NODE_IP"] = "5.4.3.2"

		test := func(d DownwardAPI) {
			m := map[string]func() string{
				"POD_IP":        d.PodIp,
				"POD_NAME":      d.PodName,
				"POD_NAMESPACE": d.PodNamespace,
				"POD_UID":       d.PodUID,
				"POD_SVCACCNT":  d.PodSvcAccount,
				"NODE_NAME":     d.NodeName,
				"NODE_IP":       d.NodeIp,
			}
			for k, v := range m {
				Expect(env[k]).To(Equal(v()))
			}
		}

		res := RetrieveDownwardAPIFrom(reader, envreader)
		test(res)
	})

	It("should detect when var is needed", func() {
		var downward TestWhichIsNeedDownwardAPI

		ExpectSet(&downward.IsNodeIp, downward.NodeIp)
		ExpectSet(&downward.IsNodeName, downward.NodeName)

		ExpectSet(&downward.IsPodIp, downward.PodIp)
		ExpectSet(&downward.IsPodName, downward.PodName)
		ExpectSet(&downward.IsPodNamespace, downward.PodNamespace)
		ExpectSet(&downward.IsPodSvcAccount, downward.PodSvcAccount)
		ExpectSet(&downward.IsPodUID, downward.PodUID)
		ExpectSetMap(&downward.IsPodLabels, downward.PodLabels)
		ExpectSetMap(&downward.IsPodAnnotations, downward.PodAnnotations)

	})
})

func ExpectSet(b *bool, getfunc func() string) {
	Expect(*b).To(BeFalse())
	getfunc()
	Expect(*b).To(BeTrue())
}

func ExpectSetMap(b *bool, getfunc func() map[string]string) {
	Expect(*b).To(BeFalse())
	getfunc()
	Expect(*b).To(BeTrue())
}
