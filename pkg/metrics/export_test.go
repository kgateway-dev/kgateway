package metrics

// ResetScoped forgets every scoped metric registered so far, so each test sweeps only its own.
func ResetScoped() {
	scopedLock.Lock()
	defer scopedLock.Unlock()

	scoped = nil
}
