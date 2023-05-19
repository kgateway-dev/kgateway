// helper script designed to...
//   1. read in all failed json artifacts from a matrix of github jobs
//   2. assemble (and send) a cohesive slack notification
//
// Representative JSON which could be passed in as an argument to this script:
// 	 all failures: '[{"url":"https://github.com/solo-io/gloo/actions/runs/3886431530","name":"gateway"},{"url":"https://github.com/solo-io/gloo/actions/runs/3886431530","name":"gloo"},{"url":"https://github.com/solo-io/gloo/actions/runs/3886431530","name":"glooctl"},{"url":"https://github.com/solo-io/gloo/actions/runs/3886431530","name":"gloomtls"},{"url":"https://github.com/solo-io/gloo/actions/runs/3886431530","name":"helm"},{"url":"https://github.com/solo-io/gloo/actions/runs/3886431530","name":"ingress"},{"url":"https://github.com/solo-io/gloo/actions/runs/3886431530","name":"upgrade"}]'
// 	 all successes: '[]'
//
// Example usage:
// 	 export PARENT_JOB_URL=https://github.com/solo-io/gloo/actions/runs/${{github.run_id}}
// 	 export PREAMBLE="Gloo nightlies (dev)"
// 	 export SLACKBOT_BEARER=${{ secrets.SLACKBOT_BEARER }}
// 	 export SLACK_CHANNEL=C0314KESVNV
// 	 test_results="$(cat */test-out.json | jq -c --slurp .)"
// 	 go run .github/workflows/helpers/notify-from-json.go $test_results

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

type Job struct {
	Url  string
	Name string
}

type Payload struct {
	Channel string `json:"channel"`
	Text    string `json:"text"`
}

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
		Channel: os.ExpandEnv("$SLACK_CHANNEL"),
		Text:    os.ExpandEnv(":large_green_circle: <$PARENT_JOB_URL|$PREAMBLE> have all passed!"),
	})
}
func send_failure(jobs []Job) {
	text := fmt.Sprintf(":red_circle: <$PARENT_JOB_URL|$PREAMBLE> have failed in %v test suites: (", len(jobs))
	for _, job := range jobs {
		text += fmt.Sprintf("<%s|%s>, ", job.Url, job.Name)
	}
	text = text[:len(text)-2] + ")"

	message_slack(Payload{
		Channel: os.ExpandEnv("$SLACK_CHANNEL"),
		Text:    os.ExpandEnv(text),
	})
}

func message_slack(data Payload) {
	payloadBytes, err := json.Marshal(data)
	if err != nil {
		panic(err)
	}
	body := bytes.NewReader(payloadBytes)

	req, err := http.NewRequest("POST", "https://slack.com/api/chat.postMessage", body)
	if err != nil {
		panic(err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", os.ExpandEnv("Bearer $SLACKBOT_BEARER"))

	var netClient = &http.Client{
		Timeout: time.Second * 10,
	}

	resp, err := netClient.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
}
