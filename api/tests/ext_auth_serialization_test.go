package tests

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
)

func TestExtAuthMetadataContextNamespacesSerialization(t *testing.T) {
	empty := []string{}

	omitted, err := json.Marshal(&kgateway.ExtAuthProvider{})
	require.NoError(t, err)
	assert.NotContains(t, string(omitted), "metadataContextNamespaces")

	optedIn, err := json.Marshal(&kgateway.ExtAuthProvider{MetadataContextNamespaces: &empty})
	require.NoError(t, err)
	assert.Contains(t, string(optedIn), `"metadataContextNamespaces":[]`)
}
