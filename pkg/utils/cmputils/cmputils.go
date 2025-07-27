package cmputils

// OnlyOneNil returns true if exactly one of the two values is nil.
// This is used to compare fields/objects that are optional and may be set to nil.
func OnlyOneNil[T any](a, b *T) bool {
	return (a == nil) != (b == nil)
}

// CompareWithNils compares two values of type T, where T is a pointer to a value.
// Values are compared using the passed compare function.
// It is safe to pass nil pointers to this function.
func CompareWithNils[T any](a, b *T, compare func(a, b *T) bool) bool {
	if OnlyOneNil(a, b) {
		return false
	}

	if a == nil && b == nil {
		return true
	}

	return compare(a, b)
}

// pointerEqual compares two pointers of type T, where T is a comparable type. It assumes that both pointers are not nil.
func pointersValsEqual[T comparable](a, b *T) bool {
	return *a == *b
}

// PointerValsEqual compares two pointers of type T, where T is a comparable type.
// It returns true if the values pointed to by the pointers are equal, false otherwise.
// It is safe to pass nil pointers to this function.
func PointerValsEqual[T comparable](a, b *T) bool {
	return CompareWithNils(a, b, pointersValsEqual[T])
}
