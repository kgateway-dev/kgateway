package errutils

import (
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"sync"
)

// AggregateConcurrent runs fns concurrently, returning a NewAggregate if there are > 1 errors
func AggregateConcurrent(funcs []func() error) error {
	// run all fns concurrently
	ch := make(chan error, len(funcs))
	var wg sync.WaitGroup
	for _, f := range funcs {
		f := f // capture f
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch <- f()
		}()
	}
	wg.Wait()
	close(ch)
	// collect up and return errors
	errs := []error{}
	for err := range ch {
		if err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 1 {
		return utilerrors.NewAggregate(errs)
	} else if len(errs) == 1 {
		return errs[0]
	}
	return nil
}
