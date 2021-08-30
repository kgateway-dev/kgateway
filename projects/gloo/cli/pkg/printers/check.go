package printers

import (
	"encoding/json"
	"fmt"
)

type CheckResponse struct {
	Resource []CheckStatus `json:"resources"`
	Errors   []string      `json:"errors"`
}
type CheckStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

func PrintChecks(CheckResponse CheckResponse, outputType OutputType) error {
	if outputType == JSON {

		cr, err := json.Marshal(CheckResponse)
		if err != nil {
			fmt.Println(err)
			return err
		}
		fmt.Println(string(cr))
	}
	return nil
}
