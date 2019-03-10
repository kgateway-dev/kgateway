package services

const (
	testrunner = "testrunner"
)

// deprecated: never used in gloo, use func in go-utils
//func SetupKubeForTest(namespace string) error {
//	context := os.Getenv("KUBECTL_CONTEXT")
//	if context == "" {
//		current, err := KubectlOut("config", "current-context")
//		if err != nil {
//			return errors.Wrap(err, "getting currrent context")
//		}
//		context = strings.TrimSuffix(current, "\n")
//	}
//	// TODO(yuval-k): this changes the context for the user? can we do this less intrusive? maybe add it to
//	// each kubectl command?
//	if err := Kubectl("config", "set-context", context, "--namespace="+namespace); err != nil {
//		return errors.Wrap(err, "setting context")
//	}
//	return Kubectl("create", "namespace", namespace)
//}
//
//// deprecated: never used in gloo, use func in go-utils
//func TeardownKube(namespace string) error {
//	return Kubectl("delete", "namespace", namespace)
//}
//
//// deprecated: never used in gloo, use func in go-utils
//func Kubectl(args ...string) error {
//	cmd := exec.Command("kubectl", args...)
//	log.Debugf("k command: %v", cmd.Args)
//	cmd.Env = os.Environ()
//	// disable DEBUG=1 from getting through to kube
//	for i, pair := range cmd.Env {
//		if strings.HasPrefix(pair, "DEBUG") {
//			cmd.Env = append(cmd.Env[:i], cmd.Env[i+1:]...)
//			break
//		}
//	}
//	cmd.Stdout = ginkgo.GinkgoWriter
//	cmd.Stderr = ginkgo.GinkgoWriter
//	return cmd.Run()
//}
//
//func KubectlOut(args ...string) (string, error) {
//	cmd := exec.Command("kubectl", args...)
//	cmd.Env = os.Environ()
//	// disable DEBUG=1 from getting through to kube
//	for i, pair := range cmd.Env {
//		if strings.HasPrefix(pair, "DEBUG") {
//			cmd.Env = append(cmd.Env[:i], cmd.Env[i+1:]...)
//			break
//		}
//	}
//	out, err := cmd.CombinedOutput()
//	if err != nil {
//		err = fmt.Errorf("%s (%v)", out, err)
//	}
//	return string(out), err
//}
//
//func KubectlOutAsync(args ...string) (*bytes.Buffer, chan struct{}, error) {
//	cmd := exec.Command("kubectl", args...)
//	cmd.Env = os.Environ()
//	// disable DEBUG=1 from getting through to kube
//	for i, pair := range cmd.Env {
//		if strings.HasPrefix(pair, "DEBUG") {
//			cmd.Env = append(cmd.Env[:i], cmd.Env[i+1:]...)
//			break
//		}
//	}
//	buf := &bytes.Buffer{}
//	cmd.Stdout = buf
//	cmd.Stderr = buf
//	err := cmd.Start()
//	if err != nil {
//		err = fmt.Errorf("%s (%v)", buf.Bytes(), err)
//	}
//	done := make(chan struct{})
//	go func() {
//		select {
//		case <-done:
//			cmd.Process.Kill()
//		}
//	}()
//	return buf, done, err
//}
//
//// WaitPodsRunning waits for all pods to be running
//func WaitPodsRunning(podNames ...string) error {
//	finished := func(output string) bool {
//		return strings.Contains(output, "Running")
//	}
//	for _, pod := range podNames {
//		if err := WaitPodStatus(pod, "Running", finished); err != nil {
//			return err
//		}
//	}
//	return nil
//}
//
//// waitPodsTerminated waits for all pods to be terminated
//func WaitPodsTerminated(podNames ...string) error {
//	for _, pod := range podNames {
//		finished := func(output string) bool {
//			return !strings.Contains(output, pod)
//		}
//		if err := WaitPodStatus(pod, "terminated", finished); err != nil {
//			return err
//		}
//	}
//	return nil
//}
//
//// TestRunner executes a command inside the TestRunner container
//func TestRunner(args ...string) (string, error) {
//	args = append([]string{"exec", "-i", testrunner, "--"}, args...)
//	return KubectlOut(args...)
//}
//
//// TestRunnerAsync executes a command inside the TestRunner container
//// returning a buffer that can be read from as it executes
//func TestRunnerAsync(args ...string) (*bytes.Buffer, chan struct{}, error) {
//	args = append([]string{"exec", "-i", testrunner, "--"}, args...)
//	return KubectlOutAsync(args...)
//}
//
//func WaitPodStatus(pod, status string, finished func(output string) bool) error {
//	timeout := time.Second * 20
//	interval := time.Millisecond * 1000
//	tick := time.Tick(interval)
//
//	log.Debugf("waiting %v for pod %v to be %v...", timeout, pod, status)
//	for {
//		select {
//		case <-time.After(timeout):
//			return fmt.Errorf("timed out waiting for %v to be %v", pod, status)
//		case <-tick:
//			out, err := KubectlOut("get", "pod", "-l", "gloo="+pod)
//			if err != nil {
//				return fmt.Errorf("failed getting pod: %v", err)
//			}
//			if strings.Contains(out, "CrashLoopBackOff") {
//				out = KubeLogs(pod)
//				return errors.Errorf("%v in crash loop with logs %v", pod, out)
//			}
//			if strings.Contains(out, "ErrImagePull") || strings.Contains(out, "ImagePullBackOff") {
//				out, _ = KubectlOut("describe", "pod", "-l", "gloo="+pod)
//				return errors.Errorf("%v in ErrImagePull with description %v", pod, out)
//			}
//			if finished(out) {
//				return nil
//			}
//		}
//	}
//}
//
//func KubeLogs(pod string) string {
//	out, err := KubectlOut("logs", "-l", "gloo="+pod)
//	if err != nil {
//		out = err.Error()
//	}
//	return out
//}
//
//func WaitNamespaceStatus(namespace, status string, finished func(output string) bool) error {
//	timeout := time.Second * 20
//	interval := time.Millisecond * 1000
//	tick := time.Tick(interval)
//
//	log.Debugf("waiting %v for namespace %v to be %v...", timeout, namespace, status)
//	for {
//		select {
//		case <-time.After(timeout):
//			return fmt.Errorf("timed out waiting for %v to be %v", namespace, status)
//		case <-tick:
//			out, err := KubectlOut("get", "namespace", namespace)
//			if err != nil {
//				return fmt.Errorf("failed getting pod: %v", err)
//			}
//			if finished(out) {
//				return nil
//			}
//		}
//	}
//}
