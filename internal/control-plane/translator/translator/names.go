package translator

// for future-proofing possible safety issues with bad upstream names
func clusterName(upstreamName string) string {
	return upstreamName
}

// for future-proofing possible safety issues with bad virtualservice names
func virtualHostName(virtualServiceName string) string {
	return virtualServiceName
}

