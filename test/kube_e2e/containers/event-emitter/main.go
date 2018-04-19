package main

import (
	"flag"

	"time"

	"log"

	"net/http"

	"github.com/solo-io/gloo-sdk-go/events"
)

var envoyAddr string
var eventData string
var topic string

func main() {
	flag.StringVar(&envoyAddr, "envoy", "http://test-ingress:8080", "address to send events (envoy gateway)")
	flag.StringVar(&eventData, "data", "what an event!", "data to send in the event")
	flag.StringVar(&topic, "topic", "test-topic", "topic to publish events to")
	flag.Parse()
	emitter := events.NewGloo(envoyAddr, nil).Emitter("test-source")

	log.Printf("waiting")
	start := make(chan struct{})
	go func() {
		http.HandleFunc("/start", func(writer http.ResponseWriter, request *http.Request) {
			start <- struct{}{}
			log.Printf("started")
		})
		http.ListenAndServe(":8080", nil)
	}()

	<-start

	for t := range time.Tick(time.Second) {
		if err := emitter.Emit(topic, data{
			Time:    t,
			Message: eventData,
		}); err != nil {
			log.Print("error emitting: %v", err)
		} else {
			log.Printf("successful emit")
		}
	}
}

type data struct {
	Time    time.Time `json:"time"`
	Message string    `json:"message"`
}
