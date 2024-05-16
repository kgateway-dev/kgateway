package utils

func PointerTo[T any](inp T) *T {
	return &inp
}
