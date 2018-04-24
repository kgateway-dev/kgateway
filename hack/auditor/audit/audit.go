package audit

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/gogo/protobuf/proto"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/log"
)

const (
	pathUpstream = "/upstreams"
	pathVhost    = "/virtualservices"
)

const (
	CrudOperationCREATE = "CREATE"
	CrudOperationUPDATE = "UPDATE"
	CrudOperationDELETE = "DELETE"
)

const (
	// the type of operation done on the object
	HeaderOperation = "x-gloo-event-operation"
	// the source
	HeaderSource = "x-gloo-event-source"
)

type EventMeta struct {
	Operation string
	Source    string
}

const addr = "http://auditor.default.svc.cluster.local:8080"

func EmitEvent(operation string, item v1.ConfigObject) error {
	body, err := proto.Marshal(item)
	if err != nil {
		return err
	}
	var path string
	switch item.(type) {
	case *v1.Upstream:
		path = pathUpstream
	case *v1.VirtualService:
		path = pathVhost
	default:
		panic("bad input")
	}
	req, err := http.NewRequest("POST", addr+path, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set(HeaderOperation, operation)
	req.Header.Set(HeaderSource, getSource())

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	if res.StatusCode != 200 {
		return fmt.Errorf("request failed: %v", res)
	}
	return nil
}

func getSource() string {
	h, err := os.Hostname()
	if err != nil {
		panic(err)
	}
	return h
}

func NewServeMux() *http.ServeMux {
	m := http.NewServeMux()
	m.HandleFunc(pathUpstream, upstreamsHandler)
	m.HandleFunc(pathVhost, vServicesHandler)
	return m
}

type logEvent struct {
	event EventMeta
	obj   v1.ConfigObject
}

func upstreamsHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("new us %v", r.Header)
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
	}
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
	defer r.Body.Close()
	var us v1.Upstream
	err = proto.Unmarshal(body, &us)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
	log.Printf("%v", logEvent{
		obj: &us,
		event: EventMeta{
			Operation: r.Header.Get(HeaderOperation),
			Source:    r.Header.Get(HeaderSource),
		},
	})
}

func vServicesHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("new vs %v", r.Header)
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
	}
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
	defer r.Body.Close()
	var vs v1.VirtualService
	err = proto.Unmarshal(body, &vs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
	log.Printf("%v", logEvent{
		obj: &vs,
		event: EventMeta{
			Operation: r.Header.Get(HeaderOperation),
			Source:    r.Header.Get(HeaderSource),
		},
	})
}
