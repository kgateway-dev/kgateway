package httpprotocolvalidation

const (
	MinWindowSize        = 65535
	MaxWindowSize        = 2147483647
	MinConcurrentStreams = 1
	MaxConcurrentStreams = 2147483647
)

func ValidateWindowSize(size uint32) bool {
	if size < MinWindowSize || size > MaxWindowSize {
		return false
	}
	return true
}

func ValidateConcurrentStreams(size uint32) bool {
	if size < MinConcurrentStreams || size > MaxConcurrentStreams {
		return false
	}
	return true
}
