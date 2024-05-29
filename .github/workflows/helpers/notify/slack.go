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
//	 results ='{ "end_to_end_tests_main": { "result": "success", "outputs": {} }}'
// 	go run .github/workflows/helpers/notify/slack.go $results

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	postMessageEndpoint = "https://slack.com/api/chat.postMessage"

	resultCancelled = "cancelled"
	resultFailure   = "failure"
	resultSkipped   = "skipped"
	resultSuccess   = "success"
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

	var requiredJobResults map[string]GithubJobResult
	err := json.Unmarshal([]byte(requiredJobs), &requiredJobResults)
	if err != nil {
		panic(err)
	}

	unsuccessfulJobs := getUnsuccessfulJobsFromResults(requiredJobResults)
	if len(unsuccessfulJobs) > 0 {
		sendFailure(unsuccessfulJobs)
	} else {
		sendSuccess()
	}
}

// getUnsuccessfulJobsFromResults processes a set of job results,
// and returns a map containing the names of any jobs that did not succeed, keyed by the failure state
// If all jobs were successful, the returned map is empty
func getUnsuccessfulJobsFromResults(requiredJobResults map[string]GithubJobResult) map[string][]string {
	unsuccessfulJobsByType := make(map[string][]string)

	for jobName, requiredJobResult := range requiredJobResults {
		switch requiredJobResult.Result {
		case resultFailure:
			fallthrough
		case resultCancelled:
			jobsForType := unsuccessfulJobsByType[requiredJobResult.Result]
			unsuccessfulJobsByType[requiredJobResult.Result] = append(jobsForType, jobName)
			continue

		case resultSuccess:
			// We don't report on individuals jobs that were successful
			continue

		case resultSkipped:
			// We don't report on individuals jobs that were skipped
			fallthrough

		default:
			continue
		}
	}

	return unsuccessfulJobsByType
}

func sendSuccess() {
	mustSendSlackText(":large_green_circle: <$PARENT_JOB_URL|$PREAMBLE> have all passed!")
}

func sendFailure(unsuccessfulJobs map[string][]string) {
	var failureReasons []string

	for reason, jobs := range unsuccessfulJobs {
		if len(jobs) == 0 {
			continue
		}
		reasonBuilder := strings.Builder{}
		reasonBuilder.WriteString(fmt.Sprintf("*%s*: ", strings.ToUpper(reason)))
		reasonBuilder.WriteString(strings.Join(jobs, ","))

		failureReasons = append(failureReasons, reasonBuilder.String())
	}

	text := fmt.Sprintf(":red_circle: <$PARENT_JOB_URL|$PREAMBLE> have failed some jobs: %s", strings.Join(failureReasons, "\n"))
	mustSendSlackText(text)
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
