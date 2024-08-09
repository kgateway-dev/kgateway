package license

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/spf13/cobra"
)

type AddOn struct {
	Addon       int    `json:"Addon"`
	ExpiresAt   int64  `json:"ExpiresAt"`
	LicenseType string `json:"LicenseType"`
}

type LicenseClaims struct {
	AddOns         []AddOn `json:"addOns"`
	ExpirationDate int64   `json:"exp"`
	CreationDate   int64   `json:"iat"`
	LicenseType    string  `json:"lt"`
	Product        string  `json:"product"`
	jwt.RegisteredClaims
}

type LicenseLegacyClaims struct {
	AddOns  string `json:"addOns"`
	Exp     int64  `json:"exp"`
	Iat     int64  `json:"iat"`
	Key     string `json:"k"`
	LicType string `json:"lt"`
	Product string `json:"product"`
	jwt.RegisteredClaims
}

func License(opts *options.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "validate",
		Aliases: []string{"v", "validate"},
		Short:   "Check Gloo Gateway License Validity",
		Long: "Checking Gloo Gateway license Validity.\n\n" +
			"" +
			"Usage: `glooctl license validate [--license-key license-key]`",

		RunE: func(cmd *cobra.Command, args []string) error {
			licenseKey := opts.ValidateLicense.LicenseKey
			if strings.Count(licenseKey, ".") == 1 {
				return validateLegacyLicense(opts.ValidateLicense.LicenseKey)
			}
			return validateLicense(opts.ValidateLicense.LicenseKey)

		}}
	flags := cmd.Flags()
	flagutils.AddLicenseValidationFlag(flags, &opts.ValidateLicense.LicenseKey)
	cmd.MarkFlagRequired(flagutils.LicenseFlag)
	return cmd
}

func validateLicense(licenseKey string) error {
	var licenseClaims LicenseClaims

	_, _, err := new(jwt.Parser).ParseUnverified(licenseKey, &licenseClaims)
	if err != nil {
		return fmt.Errorf("can't parse license key")
	}
	fmt.Printf(formatLicenseDetail(licenseClaims.CreationDate, licenseClaims.ExpirationDate, licenseClaims.Product, licenseClaims.LicenseType == "trial"))
	return nil
}

func validateLegacyLicense(licenseKey string) error {
	var licenseLegacyClaim LicenseLegacyClaims
	var standardizedLicenseKey = base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`)) + "." + licenseKey
	_, _, err := new(jwt.Parser).ParseUnverified(standardizedLicenseKey, &licenseLegacyClaim)
	if err != nil {
		return fmt.Errorf("can't parse license key")
	}
	fmt.Printf(formatLicenseDetail(licenseLegacyClaim.Iat, licenseLegacyClaim.Exp, licenseLegacyClaim.Product, licenseLegacyClaim.LicType == "trial"))
	return nil
}

func formatLicenseDetail(creationTime int64, expirationTime int64, product string, isTrial bool) string {
	var productName = "unknown"
	switch product {
	case "gloo":
		productName = "Gloo Gateway"
	}
	var res = ""
	if isTrial {
		res += fmt.Sprintln("This a trial license for", productName)
	} else {
		res += fmt.Sprintln("This an enterprise license for", productName)
	}
	res += fmt.Sprintln("This license was created on:", time.Unix(creationTime, 0))
	if expirationTime < time.Now().Unix() {
		res += fmt.Sprintln("This license is expired since:", time.Unix(expirationTime, 0))
	} else {
		res += fmt.Sprintln("This license is valid until:", time.Unix(expirationTime, 0))
	}

	return res
}
