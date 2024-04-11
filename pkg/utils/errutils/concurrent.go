package errutils

import (
	"sync"

	"k8s.io/apimachinery/pkg/util/errors"
)

// AggregateConcurrent runs fns concurrently, returning a NewAggregate if there are > 0 errors
func AggregateConcurrent(funcs []func() error) errors.Aggregate {
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
	if len(errs) > 0 {
		return errors.NewAggregate(errs)
	}
	return nil
}
