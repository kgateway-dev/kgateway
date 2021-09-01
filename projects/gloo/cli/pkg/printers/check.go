package printers

import (
	"encoding/json"
	"fmt"
	"strings"
)

type CheckResult struct {
	Resources []CheckStatus `json:"resources"`
	Messages  []string      `json:"messages"`
	Errors    []string      `json:"errors"`
}
type CheckStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

var (
	checkResult *CheckResult
)

func AppendResponse(name string, status string, message string, err string, outputType OutputType) {

	if outputType.IsTable() {

		if name != "" && status == ""{
			fmt.Printf(name)
		} else if status != "" {
			fmt.Printf(status)
		} else if message != "" {
			fmt.Printf(message)
		} 
	} else if outputType.IsJSON() {

		if checkResult == nil {
			checkResult = new(CheckResult)
		}

		if name != "" && status == "" {
			cr := CheckStatus{Name: sanitizeName(name)}
			checkResult.Resources = append(checkResult.Resources, cr)
		} else if name != "" && status != "" {
			for i := range checkResult.Resources {
				if checkResult.Resources[i].Name == name {
					checkResult.Resources[i].Status = sanitizeStatus(status)
					break
				}
			}
		} else if message != "" {
			checkResult.Messages = append(checkResult.Messages, strings.ReplaceAll(message, "\n", ""))
		} else if err != "" {
			checkResult.Errors = append(checkResult.Errors, strings.ReplaceAll(err, "\n", ""))
		}
	}
}

func PrintChecks() error {

	cr, err := json.Marshal(checkResult)
	if err != nil {
		fmt.Println(err)
		return err
	}
	fmt.Println(string(cr))

	return nil
}

func sanitizeName(name string) string {
	return strings.ReplaceAll(strings.ReplaceAll(name, "Checking ", ""), "... ", "")
}
func sanitizeStatus(status string) string {
	return strings.ReplaceAll(status, "\n", "")
}
