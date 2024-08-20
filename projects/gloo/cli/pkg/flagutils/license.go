package flagutils

import "github.com/spf13/pflag"

const (
	LicenseFlag = "license-key"
)

func AddLicenseValidationFlag(set *pflag.FlagSet, strptr *string) {
	set.StringVarP(strptr, LicenseFlag, "", "", "license key to validate")
}
