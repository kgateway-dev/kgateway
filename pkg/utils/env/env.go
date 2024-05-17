package env

import "os"

// GetOrDefault returns the value of the environment variable for the given key,
// or the default value if the environment variable is not set.
func GetOrDefault(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
