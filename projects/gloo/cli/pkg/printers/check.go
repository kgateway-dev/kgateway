package printers

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type CheckPrinters interface {
	AppendCheck(name string)
	AppendStatus(name string, status string)
	AppendMessage(message string)
	AppendError(err string)
	PrintChecks() error
	NewCheckResult() CheckResult
}

type CheckResult struct {
	Resources []CheckStatus `json:"resources"`
	Messages  []string      `json:"messages"`
	Errors    []string      `json:"errors"`
}
type CheckStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type P struct {
	OutputType  OutputType
	CheckResult *CheckResult
}

func (p P) AppendCheck(name string) {
	if p.OutputType.IsTable() {
		fmt.Printf(name)
	} else if p.OutputType.IsJSON() {
		cr := CheckStatus{Name: sanitizeName(name)}
		p.CheckResult.Resources = append(p.CheckResult.Resources, cr)
	}
}

func (p P) AppendStatus(name string, status string) {

	if p.OutputType.IsTable() {
		fmt.Printf(status + "\n")
	} else if p.OutputType.IsJSON() {
		for i := range p.CheckResult.Resources {
			if p.CheckResult.Resources[i].Name == name {
				p.CheckResult.Resources[i].Status = (status)
				break
			}
		}
	}
}

func (p P) AppendMessage(message string) {
	if p.OutputType.IsTable() {
		fmt.Printf(message + "\n")
	} else if p.OutputType.IsJSON() {
		p.CheckResult.Messages = append(p.CheckResult.Messages, strings.ReplaceAll(message, "\n", ""))
	}
}

func (p P) AppendError(err string) {
	if p.OutputType.IsTable() {
		// errors are returned by the root cmd, no need to print them here
		// fmt.Printf(err)
	} else if p.OutputType.IsJSON() {
		p.CheckResult.Errors = append(p.CheckResult.Errors, err)
	}
}

func (p P) PrintChecks(w io.Writer) {

	err := json.NewEncoder(w).Encode(p.CheckResult)
	if err != nil {
		fmt.Print(err)
	}
	fmt.Print(w)
}

func (p P) NewCheckResult() *CheckResult {

	if p.OutputType.IsJSON() {
		return new(CheckResult)
	}

	return nil
}

// We must sanitze the name for json formatting because the name comes in as "Checking deployments..."
// and we just require the type "deployments"
func sanitizeName(name string) string {
	return strings.ReplaceAll(strings.ReplaceAll(name, "Checking ", ""), "... ", "")
}
