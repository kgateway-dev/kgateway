package modfile

import (
	"encoding/json"
	"os"
	"os/exec"
)

type GoPackage struct {
	Path     string `json:"Path"`
	Version  string `json:"Version"`
	Indirect bool   `json:"Indirect"`
}

type ReplacedGoPackage struct {
	Old GoPackage `json:"Old"`
	New GoPackage `json:"New"`
}

type AllPackages struct {
	Go      string              `json:"Go"`
	Require []GoPackage         `json:"Require"`
	Replace []ReplacedGoPackage `json:"Replace"`
}

func GetCmdOutput(cmd []string) ([]byte, error) {
	command := exec.Command(cmd[0], cmd[1:]...)
	command.Stderr = os.Stderr
	return command.Output()
}

// Parse returns the go.mod dependencies in a structured format
// It is an alternative to using https://pkg.go.dev/golang.org/x/mod which frequently
// contains CVEs and must be updated
func Parse() (AllPackages, error) {
	allPackages := AllPackages{}

	depList, err := GetCmdOutput([]string{"go", "mod", "edit", "-json"})
	if err != nil {
		return allPackages, err
	}

	unmarshalErr := json.Unmarshal(depList, &allPackages)
	return allPackages, unmarshalErr
}
