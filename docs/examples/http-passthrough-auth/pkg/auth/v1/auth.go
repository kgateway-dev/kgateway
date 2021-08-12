package v1

import (
	"net/http"
)

type HttpPassthroughService struct{}

func (h *HttpPassthroughService) StartServer() {
	handler := func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(200)
	}
	http.ListenAndServe("127.0.0.1:9001", http.HandlerFunc(handler))
}
