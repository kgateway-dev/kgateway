package services

import (
	"os"
	"strings"
	"sync"

	"github.com/onsi/gomega"
	errors "github.com/rotisserie/eris"
	"github.com/solo-io/gloo/test/testutils"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	// _loggingConfigRegex is the format of the string that can be passed to configure the log level of services
	// It is currently unused, but is here for reference
	// In general, we try to use the name of the deployment, e.g. gateway-proxy, gloo, discovery, etc.
	// for the name of the service. To confirm the name of the service that is being used, check the
	// invocation for the given service
	_loggingConfigRegex = "service:logLevel,service:logLevel"
	pairSeparator       = ","
	nameValueSeparator  = ":"
)

var (
	singletonMutex       = &sync.RWMutex{}
	logProviderSingleton *logProvider
)

func init() {
	LoadUserDefinedLogLevelFromEnv()
}

func LoadUserDefinedLogLevelFromEnv() {
	LoadUserDefinedLogLevel(os.Getenv(testutils.ServiceLogLevel))
}

func LoadUserDefinedLogLevel(userDefinedLogLevel string) {
	singletonMutex.Lock()
	defer singletonMutex.Unlock()

	serviceLogPairs := strings.Split(userDefinedLogLevel, pairSeparator)
	logProviderSingleton = &logProvider{
		defaultLogLevel: zapcore.InfoLevel,
		serviceLogLevel: make(map[string]zapcore.Level, len(serviceLogPairs)),
	}

	for _, serviceLogPair := range serviceLogPairs {
		nameValue := strings.Split(serviceLogPair, nameValueSeparator)
		if len(nameValue) != 2 {
			continue
		}

		name := nameValue[0]
		logLevelStr := nameValue[1]
		logLevel, err := zapcore.ParseLevel(logLevelStr)
		// We intentionally error loudly here
		// This will occur if the user passes an invalid log level string
		if err != nil {
			panic(errors.Wrapf(err, "invalid log level string: %s", logLevelStr))
		}

		logProviderSingleton.serviceLogLevel[name] = logLevel
	}
}

// GetLogLevel returns the log level for the given service
// In general, we try to use the name of the deployment, e.g. gateway-proxy, gloo, discovery, etc.
// for the name of the service. To confirm the name of the service that is being used, check the
// invocation for the given service
func GetLogLevel(serviceName string) zapcore.Level {
	singletonMutex.RLock()
	defer singletonMutex.RUnlock()
	return logProviderSingleton.GetLogLevel(serviceName)
}

// IsDebugLogLevel returns true if the given service is logging at the debug level
func IsDebugLogLevel(serviceName string) bool {
	logLevel := GetLogLevel(serviceName)
	return logLevel == zapcore.DebugLevel
}

// MustGetSugaredLogger returns a sugared logger for the given service
// This logger is configured with the appropriate log level
func MustGetSugaredLogger(serviceName string) *zap.SugaredLogger {
	logLevel := GetLogLevel(serviceName)

	config := zap.NewProductionConfig()
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	config.Level.SetLevel(logLevel)

	logger, err := config.Build()
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "failed to build logger")

	return logger.Sugar()
}

// logProvider is a helper for managing the log level of multiple services
type logProvider struct {
	defaultLogLevel zapcore.Level

	serviceLogLevel map[string]zapcore.Level
}

func (l *logProvider) GetLogLevel(serviceName string) zapcore.Level {
	logLevel, ok := l.serviceLogLevel[serviceName]
	if !ok {
		return l.defaultLogLevel
	}
	return logLevel
}
