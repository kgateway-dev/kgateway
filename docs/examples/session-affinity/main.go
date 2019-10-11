package main

import (
	"fmt"
	"net/http"
	"os"
)

func main() {
	if err := App(); err != nil {
		os.Exit(1)
	}
}

var (
	countUrl = "/count"
	helpMsg  = fmt.Sprintf(`Simple counter app for testing Gloo

%v - reports number of times the %v path was queried`, countUrl, countUrl)
)

func App() error {
	count := 0
	http.HandleFunc(countUrl, func(w http.ResponseWriter, r *http.Request) {
		count++
		if _, err := fmt.Fprint(w, count); err != nil {
			fmt.Printf("error with request: %v\n", err)
		}
	})
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if _, err := fmt.Fprint(w, helpMsg); err != nil {
			fmt.Printf("error with request: %v\n", err)
		}
	})
	return http.ListenAndServe("0.0.0.0:8080", nil)

}
