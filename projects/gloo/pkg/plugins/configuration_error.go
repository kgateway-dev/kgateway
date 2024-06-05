package plugins

// ConfigurationError is an interface for errors that can be returned by plugins.
// In Gloo Gateway, an invalid state of translation (which results in a Go error, returned by our plugins)
// can be interpreted as either:
//   - Error (this requires user intervention)
//   - Warning (this _may_ not require user intervention)
//
// ref: https://docs.solo.io/gloo-edge/latest/guides/traffic_management/configuration_validation/
// Historically, this distinction of warnings and errors wasn't possible in plugins, and everything
// was treated as an error.
// This interface is used to distinguish errors and validation warnings.
// It is the responsibility of plugins to return errors that implement this interface if they need to
// distinguish Go errors that should be treated as warnings
//
// In the future, we may expand the methods on this interface, to allow plugins further granularity
// of reporting errors. At the time of authoring this, the only available options were: warnings/errors
type ConfigurationError interface {
	error

	// IsWarning returns true if the error is a warning.
	// Warnings can occur due to eventual consistency in resources selected by config which should not result
	// in the validation webhook rejecting the configuration.
	IsWarning() bool
}

var _ ConfigurationError = new(BaseConfigurationError)

// BaseConfigurationError is a basic implementation of the ConfigurationError
type BaseConfigurationError struct {
	errString string
	isWarning bool
}

func (b BaseConfigurationError) Error() string {
	return b.errString
}

func (b BaseConfigurationError) IsWarning() bool {
	return b.isWarning
}

// NewWarningConfigurationError returns a ConfigurationError that is a warning
func NewWarningConfigurationError(message string) *BaseConfigurationError {
	return &BaseConfigurationError{
		errString: message,
		isWarning: true,
	}
}

// NewConfigurationError returns a ConfigurationError that is not a warning
func NewConfigurationError(message string) *BaseConfigurationError {
	return &BaseConfigurationError{
		errString: message,
		isWarning: false,
	}
}
