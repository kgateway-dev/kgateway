package sliceutils

func Dedupe[E comparable](in []E) []E {
	seen := map[E]struct{}{}
	out := []E{}

	for _, e := range in {
		if _, ok := seen[e]; !ok {
			out = append(out, e)
			seen[e] = struct{}{}
		}
	}

	return out
}
