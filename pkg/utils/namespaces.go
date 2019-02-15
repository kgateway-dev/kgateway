package utils

func ProcessWatchNamespaces(watchNamespaces []string, writeNamespace string) []string {

	if len(watchNamespaces) == 0 {
		return watchNamespaces
	}
	if len(watchNamespaces) == 1 && watchNamespaces[0] == "" {
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
