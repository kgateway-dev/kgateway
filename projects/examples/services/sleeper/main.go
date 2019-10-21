package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	ctx := context.Background()

	if err := run(ctx); err != nil {
		log.Fatalf("unable to run: %v", err)
	}
}

type envConfig struct {
	port string
}

const (
	envPort = "PORT"
)

func getEnvConfig() (*envConfig, error) {
	ec := &envConfig{}
	ec.port = os.Getenv(envPort)
	if ec.port == "" {
		return nil, fmt.Errorf("must specify a port using the %v environment variable", envPort)
	}
	return ec, nil
}

func run(ctx context.Context) error {
	ec, err := getEnvConfig()
	if err != nil {
		return err
	}
	sleepServer := &sleeper{}
	return http.ListenAndServe(fmt.Sprintf("0.0.0.0:%v", ec.port), sleepServer)
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
			if _, wErr := fmt.Fprintf(w, "could not parse duration request: %v\n", sleepDurationSpec); wErr != nil {
				log.Printf("unable to write error message: %v\n", wErr)
			}
		}
	}
	time.Sleep(sleepDuration)
	if _, err := fmt.Fprintf(w, "slept for %v\n", sleepDuration.String()); err != nil {
		log.Printf("unable to respond with sleep time: %v\n", err)
	}

}
