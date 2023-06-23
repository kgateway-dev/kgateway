package services_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/test/services"
	"go.uber.org/zap/zapcore"
)

var _ = FDescribe("Logging", func() {

	Context("GetLogLevel", func() {

		It("should default to info when no log level defined", func() {
			services.LoadUserDefinedLogLevel("")
			Expect(services.GetLogLevel("undefined-service")).To(Equal(zapcore.InfoLevel))
		})

		It("should return the correct log level", func() {
			services.LoadUserDefinedLogLevel("gateway-proxy:debug,gloo:error")
			Expect(services.GetLogLevel("gateway-proxy")).To(Equal(zapcore.DebugLevel))
			Expect(services.GetLogLevel("gloo")).To(Equal(zapcore.ErrorLevel))
		})

	})

	Context("IsDebugLogLevel", func() {

		It("should return true when log level is debug", func() {
			services.LoadUserDefinedLogLevel("service:debug")
			Expect(services.IsDebugLogLevel("service")).To(BeTrue())
		})

		It("should return false when log level is not debug", func() {
			services.LoadUserDefinedLogLevel("service:error")
			Expect(services.IsDebugLogLevel("service")).To(BeFalse())
		})

	})

	Context("MustGetSugaredLogger", func() {

		It("should return logger with proper log level configured", func() {
			services.LoadUserDefinedLogLevel("service:debug")
			Expect(services.MustGetSugaredLogger("service").Level()).To(Equal(zapcore.DebugLevel))
		})

	})

})
