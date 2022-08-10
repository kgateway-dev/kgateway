package utils

import (
	"os"

	"github.com/solo-io/gloo/pkg/utils/setuputils"
)

func AllNamespaces(watchNamespaces []string) bool {

	if len(watchNamespaces) == 0 {
		return true
	}
	if len(watchNamespaces) == 1 && watchNamespaces[0] == "" {
		return true
	}
	return false
}

func ProcessWatchNamespaces(watchNamespaces []string, writeNamespace string) []string {
	if AllNamespaces(watchNamespaces) {
		return watchNamespaces
	}

	var writeNamespaceProvided bool
	for _, ns := range watchNamespaces {
		if ns == writeNamespace {
			writeNamespaceProvided = true
			break
		}
	}
	if !writeNamespaceProvided {
		watchNamespaces = append(watchNamespaces, writeNamespace)
	}

	return watchNamespaces
}

func GetPodNamespace() string {
	if podNamespace := os.Getenv(setuputils.PodNamespace); podNamespace != "" {
		return podNamespace
	}
	return "gloo-system"
}
