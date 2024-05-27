// Helper script designed to send a cohesive Slack notification about the result of a collection of GitHub workflows
// This is a simpler variant of notify-from-json.go, due to some issues we
// encountered around the accuracy of the notification: https://github.com/solo-io/solo-projects/issues/5191
//
// This works by:
// 	1. Read in the list of workflow results
//  2. Send a Slack notification about the overall result
//
// Representative JSON which could be passed in as an argument to this script:
// '[{"result":"success"},{"result":"failure"}]'
//
// Example usage:
// 	 export PARENT_JOB_URL=https://github.com/solo-io/gloo/actions/runs/${{github.run_id}}
// 	 export PREAMBLE="Gloo nightlies (dev)"
// 	 export SLACKBOT_BEARER=${{ secrets.SLACKBOT_BEARER }}
// 	 export SLACK_CHANNEL=C0314KESVNV
//	 jobs='[{"result":"success"},{"result":"failure"}]'
// 	 go run .github/workflows/helpers/notify/slack.go $jobs

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

const (
	postMessageEndpoint = "https://slack.com/api/chat.postMessage"
)

type GithubJobResult struct {
	Result string `json:"result"`
}

type Payload struct {
	Channel string `json:"channel"`
	Text    string `json:"text"`
}

func main() {
	requiredJobs := os.Args[1]
	fmt.Printf("slack.go invoked with: %v", requiredJobs)

	var requiredJobResults []GithubJobResult
	err := json.Unmarshal([]byte(requiredJobs), &requiredJobResults)
	if err != nil {
		panic(err)
	}

	for _, requiredJobResult := range requiredJobResults {
		switch requiredJobResult.Result {
		case "failure":
			sendFailure()
			return

		case "success":
			continue

		default:
			continue
		}
	}

	sendSuccess()
}

func sendSuccess() {
	mustSendSlackText(":large_green_circle: <$PARENT_JOB_URL|$PREAMBLE> have all passed!")
}
func sendFailure() {
	mustSendSlackText(":red_circle: <$PARENT_JOB_URL|$PREAMBLE> have failed some jobs")
}

func mustSendSlackText(text string) {
	fmt.Printf("send slack message with text: %s", text)
	mustSendSlackMessage(Payload{
		Channel: os.ExpandEnv("$SLACK_CHANNEL"),
		Text:    os.ExpandEnv(text),
	})
}

func mustSendSlackMessage(data Payload) {
	payloadBytes, err := json.Marshal(data)
	if err != nil {
		panic(err)
	}

	req, err := http.NewRequest(http.MethodPost, postMessageEndpoint, bytes.NewReader(payloadBytes))
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
