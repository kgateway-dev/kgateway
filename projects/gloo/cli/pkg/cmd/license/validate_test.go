package license

import (
	"time"

	"github.com/golang-jwt/jwt/v4"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("License Validate", func() {

	licenseClaims := LicenseClaims{
		AddOns:         []AddOn{{Addon: 1, ExpiresAt: time.Now().Add(24 * time.Hour).Unix(), LicenseType: "trial"}},
		ExpirationDate: time.Now().Add(24 * time.Hour).Unix(),
		CreationDate:   time.Now().Add(-24 * time.Hour).Unix(),
		LicenseType:    "trial",
		Product:        "gloo",
	}

	expiredLicenseClaims := LicenseClaims{
		AddOns:         []AddOn{{Addon: 1, ExpiresAt: time.Now().Add(24 * time.Hour).Unix(), LicenseType: "trial"}},
		ExpirationDate: time.Now().Add(-24 * time.Hour).Unix(),
		CreationDate:   time.Now().Add(-48 * time.Hour).Unix(),
		LicenseType:    "ent",
		Product:        "gloo",
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, licenseClaims)
	tokenString, err := token.SignedString([]byte("secret"))

	It("should verify a valid license", func() {
		err = validateLicense(tokenString)
		Expect(err).ToNot(HaveOccurred())
	})

	It("should fail to verify an invalid license", func() {
		invalidLicenseKey := "invalid"

		err := validateLicense(invalidLicenseKey)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal("can't parse license key"))
	})

	It("should print correct values for valid license", func() {

		res := formatLicenseDetail(licenseClaims.CreationDate, licenseClaims.ExpirationDate, licenseClaims.Product, licenseClaims.LicenseType == "trial")
		Expect(res).To(ContainSubstring("This a trial license"))
		Expect(res).To(ContainSubstring("This license is valid until"))
	})

	It("should print correct values for expired license", func() {

		res := formatLicenseDetail(expiredLicenseClaims.CreationDate, expiredLicenseClaims.ExpirationDate, expiredLicenseClaims.Product, expiredLicenseClaims.LicenseType == "trial")
		Expect(res).To(ContainSubstring("This an enterprise license for Gloo Gateway"))
		Expect(res).To(ContainSubstring("This license is expired since"))
	})

})
