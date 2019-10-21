package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"
)

func main() {
	ctx := context.Background()
	if err := run(ctx); err != nil {
		log.Fatalf("unable to run: %v", err)
	}
}

func run(ctx context.Context) error {
	sleepServer := &sleeper{}
	return http.ListenAndServe("0.0.0.0:8081", sleepServer)
}

type sleeper struct{}

const (
	queryTime = "time"
)

func (s *sleeper) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var sleepDuration time.Duration
	sleepDurationSpec := r.URL.Query().Get(queryTime)
	if sleepDurationSpec != "" {
		var err error
		sleepDuration, err = time.ParseDuration(sleepDurationSpec)
		if err != nil {
			if _, wErr := fmt.Fprintf(w, "could not parse duration request: %v", sleepDurationSpec); wErr != nil {
				log.Printf("unable to write error message: %v", wErr)
			}
		}
	}
	time.Sleep(sleepDuration)
	if _, err := fmt.Fprintf(w, "slept for %v", sleepDuration.String()); err != nil {
		log.Printf("unable to respond with sleeep time: %v", err)
	}

}
