package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
)

// event is the JSON struct emitted by test2json.
// https://cs.opensource.google/go/go/+/refs/tags/go1.23.0:src/cmd/internal/test2json/test2json.go
type event struct {
	Time    *time.Time `json:",omitempty"`
	Action  string     `json:",omitempty"`
	Package string     `json:",omitempty"`
	Test    string     `json:",omitempty"`
	Elapsed float64    `json:",omitempty"`
	Output  string     `json:",omitempty"`
}

// This tool is designed to read in a test log, process it through test2json, then
// parse through the output for pass, fail, or skip outcomes and log them all together.

// This will also output a list of all leaf-node tests ran to produce the output.
// This can be used to determine if a new test was properly run or not.
func main() {
	var logFile, outFile string
	var jsonOutput bool
	flag.StringVar(&logFile, "log-file", "./_test/test_log/go-test", "point to the raw string log output of go test command")
	flag.StringVar(&outFile, "out-file", "./_test/test_log/go-test-summary", "where to place the output summary file")
	flag.BoolVar(&jsonOutput, "json", false, "output as json")
	flag.Parse()

	b, err := readTestOutput(logFile)
	if err != nil {
		log.Fatal(err)
	}

	jsonb, err := testOutputToJson(b)
	if err != nil {
		log.Fatal(err)
	}

	allEvents, err := parseTestOutput(jsonb)
	if err != nil {
		log.Fatal(err)
	}

	resultEvents := selectResultEvents(allEvents)

	leafNodeResults := selectLeafNodes(resultEvents)

	output := printResults(leafNodeResults, jsonOutput)

	writeResults(output, outFile)
}

func readTestOutput(fname string) ([]byte, error) {
	f, err := os.ReadFile(fname)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func testOutputToJson(in []byte) ([]byte, error) {
	result := &bytes.Buffer{}

	cmd := exec.Command("go", "tool", "test2json")
	cmd.Env = os.Environ()
	cmd.Stdin = bytes.NewBuffer(in)
	cmd.Stdout = result

	err := cmd.Run()
	if err != nil {
		return nil, err
	}
	return result.Bytes(), nil

}

func parseTestOutput(in []byte) ([]*event, error) {
	rawEvents := bytes.Split(in, []byte{'\n'})
	events := []*event{}
	for _, rawEvent := range rawEvents {
		if len(rawEvent) == 0 {
			continue
		}
		ev := &event{}
		if err := json.Unmarshal(rawEvent, ev); err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	return events, nil
}

func selectResultEvents(allEvents []*event) []*event {
	resultEvents := []*event{}
	for _, ev := range allEvents {
		if ev.Test != "" && (ev.Action == "pass" ||
			ev.Action == "fail" ||
			ev.Action == "skip") {
			resultEvents = append(resultEvents, ev)
		}
	}
	return resultEvents
}

func selectLeafNodes(events []*event) []*event {
	t := &multitree{}
	result := []*event{}

	for _, ev := range events {
		ev := ev
		t.pushString(ev.Test, ev)
	}

	for _, n := range t.leafNodes() {
		result = append(result, n.ev)
	}

	return result
}

func printResults(events []*event, jsonOutput bool) []byte {
	if jsonOutput {
		return printJson(events)
	}
	out := &bytes.Buffer{}
	for _, ev := range events {
		evStr := fmt.Sprintf("%s --- %s\n", strings.ToUpper(ev.Action), ev.Test)
		out.WriteString(evStr)
	}

	b := out.Bytes()
	os.Stdout.Write(b)
	return b
}
func printJson(events []*event) []byte {
	b, err := json.Marshal(events)
	if err != nil {
		log.Fatal(err)
	}

	os.Stdout.Write(b)

	return b
}

func writeResults(output []byte, filename string) {
	if err := os.WriteFile(filename, output, os.ModePerm); err != nil {
		log.Fatal(err)
	}
}
