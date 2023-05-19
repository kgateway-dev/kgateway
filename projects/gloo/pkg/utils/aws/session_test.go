package aws_test

import (
	"github.com/aws/aws-sdk-go/aws"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	. "github.com/solo-io/gloo/projects/gloo/pkg/utils/aws"
)

var _ = Describe("Session", func() {

	var (
		secrets   v1.SecretList
		secretRef *core.ResourceRef
	)

	BeforeEach(func() {
		secrets = v1.SecretList{{
			Metadata: &core.Metadata{
				Name:      "name",
				Namespace: "namespace",
			},
			Kind: &v1.Secret_Aws{
				Aws: &v1.AwsSecret{
					AccessKey: "1",
					SecretKey: "2",
				},
			},
		}}
		secretRef = &core.ResourceRef{
			Name:      "name",
			Namespace: "namespace",
		}
	})

	It("should generate the default session", func() {
		_, err := GetAwsSession(nil, nil, nil)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should generate the session with config", func() {
		session, err := GetAwsSession(nil, nil, &aws.Config{Region: aws.String("us-east-1")})
		Expect(err).NotTo(HaveOccurred())
		Expect(session.Config.Region).NotTo(Equal("us-east-1"))

	})

	It("should return a session with specified secret", func() {
		session, err := GetAwsSession(secretRef, secrets, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(session.Config.Credentials).NotTo(BeNil())
	})

	It("should error on missing secret", func() {
		secretRef.Name = "other-secret"
		_, err := GetAwsSession(secretRef, secrets, nil)
		Expect(err).To(MatchError(ContainSubstring("secrets not found for secret ref other-secret.namespace")))
	})
	It("should error on non aws secret", func() {
		secrets[0].Kind = &v1.Secret_Tls{}
		_, err := GetAwsSession(secretRef, secrets, nil)
		Expect(err).To(MatchError(ContainSubstring("provided secret is not an aws secret")))
	})

	It("should error on aws access key is not a valid string", func() {
		secrets[0].Kind = &v1.Secret_Aws{
			Aws: &v1.AwsSecret{
				AccessKey: "\xff",
				SecretKey: "2",
			},
		}
		_, err := GetAwsSession(secretRef, secrets, nil)
		Expect(err).To(MatchError("access_key not a valid string"))
	})
	It("should error on aws secret key is not a valid string", func() {
		secrets[0].Kind = &v1.Secret_Aws{
			Aws: &v1.AwsSecret{
				AccessKey: "1",
				SecretKey: "\xff",
			},
		}
		_, err := GetAwsSession(secretRef, secrets, nil)
		Expect(err).To(MatchError("secret_key not a valid string"))
	})
})
