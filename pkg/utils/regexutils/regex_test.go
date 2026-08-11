package regexutils_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/regexutils"
)

func TestCheckRegexString(t *testing.T) {
	require.NoError(t, regexutils.CheckRegexString(`^foo.*$`))
	require.Error(t, regexutils.CheckRegexString(`[[invalid`))
}
