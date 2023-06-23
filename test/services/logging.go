package services

import (
	"log"
	"os"
	"strings"

	"github.com/onsi/gomega"
	errors "github.com/rotisserie/eris"
	"github.com/solo-io/gloo/test/testutils"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	// _loggingConfigRegex is the format of the string that can be passed to configure the log level of services
	// It is currently unused, but is here for reference
	_loggingConfigRegex = "service=logLevel;service=logLevel"
	pairSeparator       = ";"
	nameValueSeparator  = "="
)

var (
	logProviderSingleton *LogProvider
)

func init() {
	loadUserDefinedLogLevel(os.Getenv(testutils.ServiceLogLevel))
}

func loadUserDefinedLogLevel(userDefinedLogLevel string) {
	serviceLogPairs := strings.Split(userDefinedLogLevel, pairSeparator)
	serviceLogLevel := make(map[string]zapcore.Level, len(serviceLogPairs))
	for _, serviceLogPair := range serviceLogPairs {
		name := strings.Split(serviceLogPair, nameValueSeparator)[0]
		logLevelStr := strings.Split(serviceLogPair, nameValueSeparator)[1]
		logLevel, err := zapcore.ParseLevel(logLevelStr)
		// We intentionally error loudly here
		// This will occur if the user passes an invalid log level string
		if err != nil {
			panic(errors.Wrapf(err, "invalid log level string: %s", logLevelStr))
		}

		serviceLogLevel[name] = logLevel
	}

	logProviderSingleton = &LogProvider{
		defaultLogLevel: zapcore.InfoLevel,
		serviceLogLevel: serviceLogLevel,
	}

	log.Printf("Log level configuration: %+v", logProviderSingleton)
}

// GetLogLevel returns the log level for the given service
// In general, we try to use the name of the deployment, e.g. gateway-proxy, gloo, discovery, etc.
// for the name of the service. To confirm the name of the service that is being used, check the
// invocation for the given service
func GetLogLevel(serviceName string) zapcore.Level {
	return logProviderSingleton.GetLogLevel(serviceName)
}

func IsDebugLogLevel(serviceName string) bool {
	logLevel := GetLogLevel(serviceName)
	return logLevel == zapcore.DebugLevel
}

func MustGetSugaredLogger(serviceName string) *zap.SugaredLogger {
	logLevel := GetLogLevel(serviceName)

	config := zap.NewProductionConfig()
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	config.Level.SetLevel(logLevel)

	logger, err := config.Build()
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "failed to build logger")

	return logger.Sugar()
}

type LogProvider struct {
	defaultLogLevel zapcore.Level

	serviceLogLevel map[string]zapcore.Level
}

func (l *LogProvider) GetLogLevel(serviceName string) zapcore.Level {
	logLevel, ok := l.serviceLogLevel[serviceName]
	if !ok {
		return l.defaultLogLevel
	}
	return logLevel
}
