package deepequaloff

import "reflect"

type Sample struct{}

func (Sample) Equals(other Sample) bool {
	return reflect.DeepEqual(Sample{}, other)
}
