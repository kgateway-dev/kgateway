package markers

type Sample struct {
	Used    int
	Missing int // want "field \"Missing\" in type \"Sample\" is not used in Equals"
	// +noKrtEquals reason: internal bookkeeping
	Ignored int
	// +krtEqualsTodo reconcile waypoint usage
	Todo int
	// TodoWithGodoc has a godoc comment in addition to the marker
	// +krtEqualsTodo reason: TODO
	TodoWithGodoc int
}

func (s Sample) Equals(other Sample) bool {
	return s.Used == other.Used
}
