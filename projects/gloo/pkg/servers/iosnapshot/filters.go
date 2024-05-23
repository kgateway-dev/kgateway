package iosnapshot

type filter interface {
	// Contains returns true if filter contains this value, false otherwise
	Contains(s string) bool
	// Exists returns true if filter exists, false otherwise
	Exists() bool
}

type filterMap map[string]bool

func newFilter(selectedFilters []string) filter {
	var fMap filterMap
	fMap = make(map[string]bool)
	for _, f := range selectedFilters {
		fMap[f] = true
	}
	return fMap
}

type Filters struct {
	namespaces    filter
	resourceTypes filter
}

func NewFilters(includedNamespaces []string) Filters {
	return Filters{
		namespaces: newFilter(includedNamespaces),
	}
}

func (f filterMap) Contains(s string) bool {
	if _, ok := f[s]; ok {
		return true
	} else {
		return false
	}
}

func (f filterMap) Exists() bool {
	if len(f) != 0 {
		return true
	} else {
		return false
	}
}
