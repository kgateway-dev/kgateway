package services

import (
	"fmt"
	"github.com/solo-io/go-utils/contextutils"
	"os"
	"os/exec"
	"strings"
	"time"

	ps "github.com/keybase/go-ps"
)

type Runner struct {
	Sourcepath    string
	ComponentName string
}

func (r *Runner) waitForExternalProcess() error {
	for {
		procs, err := ps.Processes()
		if err != nil {
			contextutils.LoggerFrom(nil).DPanic(err)
		}
		for _, proc := range procs {
			str, err := proc.Path()
			if err != nil {
				continue
			}
			if strings.Contains(str, r.Sourcepath) {
				return nil
			}

		}
		time.Sleep(time.Second)
	}
}

func (r *Runner) run(c *exec.Cmd) (*exec.Cmd, error) {
	if os.Getenv("USE_DEBUGGER_"+r.ComponentName) == "1" {
		fmt.Printf("Please run the following command in your debugger:\n"+
			"%v %v \n"+
			"CWD %v \n"+
			"looking for processes started from %v", c.Path, c.Args, c.Dir, r.Sourcepath)

		return nil, r.waitForExternalProcess()
	}
	return c, c.Start()
}
