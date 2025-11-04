package deepequalon

import "reflect"

type Sample struct{}

func (Sample) Equals(other Sample) bool {
	return reflect.DeepEqual(Sample{}, other) // want `Equals\(\) method uses reflect\.DeepEqual which is slow and ignores //\+noKrtEquals markers`
}
