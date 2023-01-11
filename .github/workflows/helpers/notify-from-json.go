// helper script designed to...
//   1. read in all failed json artifacts from a matrix of github jobs
//   2. assemble (and send) a cohesive slack notification
//
// Representative JSON which could be passed in as an argument to this script:
// 	 all failures: '[{"url":"https://github.com/solo-io/gloo/actions/runs/3886431530","name":"gateway"},{"url":"https://github.com/solo-io/gloo/actions/runs/3886431530","name":"gloo"},{"url":"https://github.com/solo-io/gloo/actions/runs/3886431530","name":"glooctl"},{"url":"https://github.com/solo-io/gloo/actions/runs/3886431530","name":"gloomtls"},{"url":"https://github.com/solo-io/gloo/actions/runs/3886431530","name":"helm"},{"url":"https://github.com/solo-io/gloo/actions/runs/3886431530","name":"ingress"},{"url":"https://github.com/solo-io/gloo/actions/runs/3886431530","name":"upgrade"}]'
// 	 all successes: '[]'
//
// Example usage:
//   test_results="'""$(cat */test-out.json | jq -c --slurp .)""'"
//   PARENT_JOB_URL="https://github.com/solo-io/gloo/actions/runs/${{github.run_id}}" SLACKBOT_BEARER=${{ secrets.SLACKBOT_BEARER }} go run .github/workflows/helpers/notify-from-json.go $test_results

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

type Job struct {
	Url  string
	Name string
}

type Payload struct {
	Channel string `json:"channel"`
	Text    string `json:"text"`
}

const SLACK_TESTING_CHANNEL = "C0314KESVNV" // # slack-integration-testing
const SLACK_CHANNEL = "C04CJMXAH7A"         // # edge-nightly-results

func main() {
	var jobs []Job
	json.Unmarshal([]byte(os.Args[1]), &jobs)

	if len(jobs) == 0 {
		send_success()
	} else {
		send_failure(jobs)
	}
}

func send_success() {
	message_slack(Payload{
		Channel: SLACK_CHANNEL, // change to SLACK_TESTING_CHANNEL to not spam standard watch channel
		Text:    os.ExpandEnv(":large_green_circle: <$PARENT_JOB_URL|Gloo OSS nightlies> have all passed!"),
	})
}
func send_failure(jobs []Job) {
	text := fmt.Sprintf(":red_circle: <$PARENT_JOB_URL|Gloo OSS nightlies> have failed in %v test suites: (", len(jobs))
	for _, job := range jobs {
		text += fmt.Sprintf("<%s|%s>, ", job.Url, job.Name)
	}
	text = text[:len(text)-2] + ")"

	message_slack(Payload{
		Channel: SLACK_CHANNEL, // change to SLACK_TESTING_CHANNEL to not spam standard watch channel
		Text:    os.ExpandEnv(text),
	})
}

func message_slack(data Payload) {
	payloadBytes, _ := json.Marshal(data)
	body := bytes.NewReader(payloadBytes)

	req, _ := http.NewRequest("POST", "https://slack.com/api/chat.postMessage", body)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", os.ExpandEnv("Bearer $SLACKBOT_BEARER"))

	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()
}
