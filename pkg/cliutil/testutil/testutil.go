package testutil

import (
	"io"
	"strings"
	"time"

	expect "github.com/Netflix/go-expect"
	"github.com/hinshun/vt10x"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/cliutil"
	"gopkg.in/AlecAivazis/survey.v1/terminal"
)

func Stdio(c *expect.Console) terminal.Stdio {
	return terminal.Stdio{c.Tty(), c.Tty(), c.Tty()}
}

func ExpectInteractive(userInput func(*Console), testCli func()) {
	c, state, err := vt10x.NewVT10XConsole()
	Expect(err).NotTo(HaveOccurred())
	defer c.Close()
	cliutil.UseStdio(Stdio(c))
	// Dump the terminal's screen.
	defer func() { GinkgoWriter.Write([]byte(expect.StripTrailingEmptyLines(state.String()))) }()

	doneC := make(chan struct{})
	go func() {
		defer GinkgoRecover()
		defer close(doneC)

		userInput(&Console{console: c})
	}()

	//	time.Sleep(time.Hour)
	go func() {
		defer GinkgoRecover()

		testCli()

		// Close the slave end of the pty, and read the remaining bytes from the master end.
		c.Tty().Close()
		<-doneC
	}()

	select {
	case <-time.After(10 * time.Second):
		c.Tty().Close()
		Fail("test timed out")
	case <-doneC:
	}
}

type Console struct {
	console *expect.Console
}

func (c *Console) ExpectString(s string) string {
	ret, err := c.console.ExpectString(s)
	Expect(err).NotTo(HaveOccurred())
	return ret
}

func (c *Console) PressDown() {
	// These codes are covered here: https://en.wikipedia.org/wiki/ANSI_escape_code
	// see "Escape sequences" and "CSI sequences"
	// 27 = Escape
	// Alternatively, you can use the values written here: gopkg.in/AlecAivazis/survey.v1/terminal/sequences.go
	// But I used the CSI as I seems to be more standard

	_, err := c.console.Write([]byte{27, '[', 'B'})
	Expect(err).NotTo(HaveOccurred())
}

func (c *Console) Esc() {
	// I grabbed this value from here: gopkg.in/AlecAivazis/survey.v1/terminal/sequences.go
	// Originally I tried to use escape codes (https://en.wikipedia.org/wiki/ANSI_escape_code)
	// but it didnt work
	_, err := c.console.Write([]byte{27})
	Expect(err).NotTo(HaveOccurred())
}

func (c *Console) SendLine(s string) int {
	ret, err := c.console.SendLine(s)
	Expect(err).NotTo(HaveOccurred())
	return ret
}

func (c *Console) ExpectEOF() string {
	ret, err := c.console.ExpectEOF()
	Expect(err).NotTo(HaveOccurred())
	return ret
}

type MockKubectl struct {
	Expected        []string
	Next            int
	StdoutLines     []string
	StdoutLineIndex int
}

func NewMockKubectl(cmds []string, stdoutLines []string) *MockKubectl {
	return &MockKubectl{
		Expected:    cmds,
		Next:        0,
		StdoutLines: stdoutLines,
	}
}

func (k *MockKubectl) Kubectl(stdin io.Reader, args ...string) error {
	// If this fails then the CLI tried to run commands we didn't account for in the mock
	Expect(k.Next < len(k.Expected)).To(BeTrue())
	Expect(stdin).To(BeNil())
	cmd := strings.Join(args, " ")
	Expect(cmd).To(BeEquivalentTo(k.Expected[k.Next]))
	k.Next = k.Next + 1
	return nil
}

func (k *MockKubectl) KubectlOut(stdin io.Reader, args ...string) ([]byte, error) {
	Expect(k.Next < len(k.Expected)).To(BeTrue(), "MockKubectl did not have a next command for KubectlOut")
	Expect(stdin).To(BeNil(), "Should have passed nil to MockKubectl.KubectlOut")
	cmd := strings.Join(args, " ")
	Expect(cmd).To(BeEquivalentTo(k.Expected[k.Next]), "Wrong next command for MockKubectl.KubectlOut")
	k.Next = k.Next + 1
	Expect(k.StdoutLineIndex < len(k.StdoutLines)).To(BeTrue(), "Mock kubectl has run out of stdout lines on command "+cmd)
	stdOutLine := k.StdoutLines[k.StdoutLineIndex]
	k.StdoutLineIndex = k.StdoutLineIndex + 1
	return []byte(stdOutLine), nil
}
