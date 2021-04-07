package secret_test

import (
	"context"
	"fmt"
	"os"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/create/secret"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/argsutils"

	"io/ioutil"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/log"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

var _ = Describe("Secret", func() {

	var (
		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		helpers.UseMemoryClients()
		ctx, cancel = context.WithCancel(context.Background())
	})

	AfterEach(func() {
		helpers.UseDefaultClients()
		cancel()
	})

	Context("Empty args and flags", func() {
		It("should give clear error message", func() {
			err := testutils.Glooctl("create secret")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(secret.EmptyCreateError))
		})
	})

	Context("AWS", func() {
		It("should error if no name provided", func() {
			err := testutils.Glooctl("create secret aws")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(argsutils.NameError))
		})

		shouldWork := func(command, namespace string) {
			err := testutils.Glooctl(command)
			Expect(err).NotTo(HaveOccurred())

			secret, err := helpers.MustSecretClient(ctx).Read(namespace, "test", clients.ReadOpts{})
			Expect(err).NotTo(HaveOccurred())

			aws := v1.AwsSecret{
				AccessKey: "foo",
				SecretKey: "bar",
			}
			Expect(*secret.GetAws()).To(Equal(aws))
		}

		It("should work", func() {
			shouldWork("create secret aws --name test --access-key foo --secret-key bar", "gloo-system")
		})

		It("can print the kube yaml as dry run", func() {
			out, err := testutils.GlooctlOut("create secret aws --dry-run --name test --access-key foo --secret-key bar")
			Expect(err).NotTo(HaveOccurred())
			Expect(out).To(ContainSubstring(`data:
  aws_access_key_id: Zm9v
  aws_secret_access_key: YmFy
metadata:
  creationTimestamp: null
  name: test
  namespace: gloo-system
type: Opaque
`))
		})

		It("can print the kube yaml as dry run with token", func() {
			out, err := testutils.GlooctlOut("create secret aws --dry-run --name test --access-key foo --secret-key bar --session-token waz")
			Expect(err).NotTo(HaveOccurred())
			Expect(out).To(ContainSubstring(`data:
  aws_access_key_id: Zm9v
  aws_secret_access_key: YmFy
  aws_session_token: d2F6
metadata:
  creationTimestamp: null
  name: test
  namespace: gloo-system
type: Opaque
`))
		})

		It("should work as subcommand", func() {
			shouldWork("create secret aws test --access-key foo --secret-key bar", "gloo-system")
		})

		It("should work in custom namespace", func() {
			shouldWork("create secret aws test --namespace custom --access-key foo --secret-key bar", "custom")
		})
	})

	Context("Azure", func() {
		It("should error if no name provided", func() {
			err := testutils.Glooctl("create secret azure")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(argsutils.NameError))
		})

		shouldWork := func(command, namespace string) {
			err := testutils.Glooctl(command)
			Expect(err).NotTo(HaveOccurred())

			secret, err := helpers.MustSecretClient(ctx).Read(namespace, "test", clients.ReadOpts{})
			Expect(err).NotTo(HaveOccurred())

			azure := v1.AzureSecret{
				ApiKeys: map[string]string{
					"foo":  "bar",
					"gloo": "baz",
				},
			}
			Expect(*secret.GetAzure()).To(Equal(azure))
		}

		It("should work", func() {
			shouldWork("create secret azure --name test --api-keys foo=bar,gloo=baz", "gloo-system")
		})

		It("can print the kube yaml in dry run", func() {
			out, err := testutils.GlooctlOut("create secret azure --dry-run --name test --name test --api-keys foo=bar,gloo=baz")
			Expect(err).NotTo(HaveOccurred())
			Expect(out).To(ContainSubstring(`data:
  azure: YXBpS2V5czoKICBmb286IGJhcgogIGdsb286IGJhego=
metadata:
  annotations:
    resource_kind: '*v1.Secret'
  creationTimestamp: null
  name: test
  namespace: gloo-system
`))
		})

		It("should work as subcommand", func() {
			shouldWork("create secret azure test --api-keys foo=bar,gloo=baz", "gloo-system")
		})

		It("should work with custom namespace", func() {
			shouldWork("create secret azure test --namespace custom --api-keys foo=bar,gloo=baz", "custom")
		})
	})

	Context("Header", func() {
		shouldWork := func(command, namespace string) {
			err := testutils.Glooctl(command)
			Expect(err).NotTo(HaveOccurred())

			secret, err := helpers.MustSecretClient(ctx).Read(namespace, "test", clients.ReadOpts{})
			Expect(err).NotTo(HaveOccurred())

			header := v1.HeaderSecret{
				Headers: map[string]string{
					"foo": "bar",
					"bat": "=b=a=z=",
				},
			}
			Expect(*secret.GetHeader()).To(Equal(header))
		}

		It("should work", func() {
			shouldWork("create secret header --name test --headers foo=bar,bat==b=a=z=", "gloo-system")
		})
	})

	Context("TLS", func() {
		It("should error if no name provided", func() {
			err := testutils.Glooctl("create secret tls")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(argsutils.NameError))
		})

		It("should work as with just root ca", func() {

			rootca := mustWriteTestFile("foo")
			err := testutils.Glooctl("create secret tls valid --namespace gloo-system --rootca " + rootca)
			Expect(err).NotTo(HaveOccurred())
			tls := v1.TlsSecret{
				RootCa: "foo",
			}

			secret, err := helpers.MustSecretClient(ctx).Read("gloo-system", "valid", clients.ReadOpts{})
			Expect(err).NotTo(HaveOccurred())
			Expect(*secret.GetTls()).To(Equal(tls))
		})

		It("should work as expected with valid and invalid input", func() {
			type keyPair struct {
				shouldPass   bool
				resourceName string
				key          string
				cert         string
			}
			keyPairTestTable := []keyPair{
				{shouldPass: true, resourceName: "valid1", key: privateKey1, cert: privateKey1Cert},
				{shouldPass: true, resourceName: "valid2", key: privateKey2, cert: privateKey2Cert},
				{shouldPass: false, resourceName: "invalid1", key: privateKey1, cert: privateKey2Cert},
			}
			for i, kp := range keyPairTestTable {
				func() {
					By(fmt.Sprintf("KeyPair test table, row %v", i))
					rootca := mustWriteTestFile("foo")
					defer os.Remove(rootca)
					privatekey := mustWriteTestFile(kp.key)
					defer os.Remove(privatekey)
					certchain := mustWriteTestFile(kp.cert)
					defer os.Remove(certchain)
					args := fmt.Sprintf(
						"create secret tls %s --namespace gloo-system --rootca %s --privatekey %s --certchain %s",
						kp.resourceName,
						rootca,
						privatekey,
						certchain)

					tls := v1.TlsSecret{
						RootCa:     "foo",
						PrivateKey: kp.key,
						CertChain:  kp.cert,
					}

					if kp.shouldPass {
						err := testutils.Glooctl(args)
						Expect(err).NotTo(HaveOccurred())

						secret, err := helpers.MustSecretClient(ctx).Read("gloo-system", kp.resourceName, clients.ReadOpts{})
						Expect(err).NotTo(HaveOccurred())
						Expect(*secret.GetTls()).To(Equal(tls))
					} else {
						err := testutils.Glooctl(args)
						Expect(err).To(HaveOccurred())

						_, err = helpers.MustSecretClient(ctx).Read("gloo-system", kp.resourceName, clients.ReadOpts{})
						Expect(err).To(HaveOccurred())
					}

				}()
			}
		})
		It("can print the kube yaml", func() {
			rootca := mustWriteTestFile("foo")
			defer os.Remove(rootca)
			privatekey := mustWriteTestFile(privateKey1)
			defer os.Remove(privatekey)
			certchain := mustWriteTestFile(privateKey1Cert)
			defer os.Remove(certchain)
			args := fmt.Sprintf(
				"create secret tls test --dry-run --name test --namespace gloo-system --rootca %s --privatekey %s --certchain %s",
				rootca,
				privatekey,
				certchain)

			out, err := testutils.GlooctlOut(args)
			Expect(err).NotTo(HaveOccurred())

			fmt.Println(out)
			Expect(out).To(ContainSubstring(`data:
  tls: Y2VydENoYWluOiB8MgoKICAtLS0tLUJFR0lOIENFUlRJRklDQVRFLS0tLS0KICBNSUlDdkRDQ0FhUUNDUURybzZaWHliaGxZREFOQmdrcWhraUc5dzBCQVFzRkFEQWdNUjR3SEFZRFZRUUREQlZ3CiAgWlhSemRHOXlaVEV1WlhoaGJYQnNaUzVqYjIwd0hoY05NVGt3TkRBMU1Ua3lPRFEyV2hjTk1qQXdOREEwTVRreQogIE9EUTJXakFnTVI0d0hBWURWUVFEREJWd1pYUnpkRzl5WlRFdVpYaGhiWEJzWlM1amIyMHdnZ0VpTUEwR0NTcUcKICBTSWIzRFFFQkFRVUFBNElCRHdBd2dnRUtBb0lCQVFES3Rmc0QvbHEvaHRHdTI4aXZ6THp6bVplOGZPaXFCbjJlCiAgTlVqOFFWT2xITG5heFRadnJxbGtUQW5Qc0R2ZEhDU1RRclpER0k1YW0xL2N0TUtvZERiZmx5Ymdza0t2WnZEaQogIDZCRjF3MXNiWjNrUEdaRXQwRW55a29pS0R2YysrWHVpTlV2TVFHMDJCMDN2NGl1cnJMYTZUTGFwQjhOV3RtUkoKICAzbi9QYmE1MDB6dSt1REY2V0ZrTlRFYXd6dU1NbDdqTFg1Z21rRFFlQlZ2WDEySVJZMEUxenlxN29UTnhzeXBwCiAgeHZrbmhBVktkTFlzS1JWQ3R3cnVDMEFDamR3WWdxNDdObjFTN2Q3YjdNOFRjUmtjc0hxcHk0ZlV2c2tpMXlrZgogIFg0NWt0M1FjSDhYRXBDTFBBNkI3NC8xWURBR0tmZGZCS1RzWmJldVV0NVEzdjh1Mi9WSkRBZ01CQUFFd0RRWUoKICBLb1pJaHZjTkFRRUxCUUFEZ2dFQkFMOG01VGpGRWI1OE1FWEtHZGJ5R1pFZFMwRnBOSStmWUJZeXhrcFU1L3ozCiAgMDZoVjJhamlzZ3ZIR3lHdW4vSExCRFh0V25iTldLcFNqaUppUzlLcGt2Nlg3M2hiYTZRM3AzcHJqZ2RYa3BTVQogIE9Ob3p3bE1NMVNNMGRqL081VlVMa2NXNHVoU1FKRXlJUlJQaUE4ZnNscVd1SWxyNUtXV1BiZElrRGV4LzlEZGYKICBvQzdEMWV4Y2xaTlZEVm1KellGU3hiMWpzL3JTc2xuMTFWSjd1eW96cGsyM2xyQVZHSXJ0ZzVYcjR2eHFVWkhVCiAgVE9lRlNWSDZMTUM1ai9GZmYrYkVCaGJQeEpBSTBQN1ZYYXBoWWgvZE15QUVxK3hSeG02c3N1Y2NnQ3l2dHRtegogICs2c1VpdnZ4YURoVUNBekFvTFNhNVhnbjVlTmRzZVB6NlBRNVZ5L1lpZGc9CiAgLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQpwcml2YXRlS2V5OiB8MgoKICAtLS0tLUJFR0lOIFBSSVZBVEUgS0VZLS0tLS0KICBNSUlFdmdJQkFEQU5CZ2txaGtpRzl3MEJBUUVGQUFTQ0JLZ3dnZ1NrQWdFQUFvSUJBUURLdGZzRC9scS9odEd1CiAgMjhpdnpMenptWmU4Zk9pcUJuMmVOVWo4UVZPbEhMbmF4VFp2cnFsa1RBblBzRHZkSENTVFFyWkRHSTVhbTEvYwogIHRNS29kRGJmbHliZ3NrS3ZadkRpNkJGMXcxc2JaM2tQR1pFdDBFbnlrb2lLRHZjKytYdWlOVXZNUUcwMkIwM3YKICA0aXVyckxhNlRMYXBCOE5XdG1SSjNuL1BiYTUwMHp1K3VERjZXRmtOVEVhd3p1TU1sN2pMWDVnbWtEUWVCVnZYCiAgMTJJUlkwRTF6eXE3b1ROeHN5cHB4dmtuaEFWS2RMWXNLUlZDdHdydUMwQUNqZHdZZ3E0N05uMVM3ZDdiN004VAogIGNSa2NzSHFweTRmVXZza2kxeWtmWDQ1a3QzUWNIOFhFcENMUEE2Qjc0LzFZREFHS2ZkZkJLVHNaYmV1VXQ1UTMKICB2OHUyL1ZKREFnTUJBQUVDZ2dFQkFMVmIvMHBoWkx0NldWdENFOWtGS2dBZjZKdWdmV0N4RWU1YjZnS1dSOG12CiAgVzdDWlJNekN6WmFJV1RiUmk1MlZNanYyTWE3eDUxcTFMQjBBTkRBV1dZbk5aK0VjVzRFbWJsbjBHcnJybnpWegogIGErSFFsQTBURHpYUldBdDh2RVJCWFJXUTdWRytTbmRPTGJKeS9YTkl3T3NJKzF0Yk1LOEIyOVFqRnVKMFZPTDcKICBDVW9HMS8wQlh3NnNsb2g5VWNvcU00blZjTmFxb2N0aTRLN2QzWjAvVDF6Snp4Nm80bzg5RHFCUm56VGowYjlPCiAgZ2Urc1JxR0Z4RWVqcjJocU1KMGpGNUJQWVVPeXQvNGt6MWdKY1JzdDBOSTlkdHR3VFBXV29kdTJhM2VRVDFTRgogIEdtWWFKcFpRQWhrblE2TGJGTUlkR3hMVWp0OUVqMEQvWVJqQXZqcnM1U0VDZ1lFQThvNU1Kb3E4Z0lNYllGdVMKICBVcFI5L3FoT2gxRWFSMUQ2U0VLQnBTdHZWWGtSUjBKSFU3N2g1Z21aWk5qemVvR3pSNXJPTjVpcTJrdUtOb21XCiAganQ0UzZCZEVQb1IrS3F4dS81NXVWaG1kclNtUC90TVhOMFhUUWE2M0hzY21XVXNhZEJtL0UxeEFhaXZZRzdXOQogIEd5enNWVytIQXBtNm1UbUwyUytZUTFhQjFlc0NnWUVBMWZKUFRpRmlmakF1aFBLT0xIUVo2V21nZEFXTkFzRGkKICBuSW9NOUJTYXhaWU5vN28ybmVzMkZJYTg4aElBdGx5Q3pMTU5JOHI3eGtEM1hKRXNXb2UrVlgxZ2JlVGpBUnBtCiAgVzZ5TE1rWXZRc0Y2VXJ0NEp3MW0yK21DYmJoL2FOZmRhTDNIckhldlU4bU1VYVA5aVJla1hZTmgyNENhTksrdgogIHQyWVJ1V3NDSndrQ2dZQmdQK01yOENXNUFVMmR3UGloV0ZkZTlENmxKNk83NVFCTUtFZjEyUFNIQUZIQTZ5WU8KICByMUpJekVwWVlGYk5xQ1lTSmZYcXplUU9WNmR5Mk1vcnl5ZkpmV0lSUk5ZajdPVG0vbUZlUFMvNmhPR2xCdkxSCiAgZGgzTWxKNEowcEQvSWZSUFdlQWV1SjYvQXNMd3kvOU1oMWtJMWdiSEcyV1dZK1dBdTRnNlFGdXBIUUtCZ1FDSwogIE9EV01JSDFsVVBOODZNZDVhTGlrMTV6VjJCQTF5eStjT29RTDNKUHhPdlFzNXMwS1VUOXJHM0ZPWXRzYTljRjcKICBSZUlqVWF3L2RSRmFPR0FUVE1kbXE4MTBzZjhHWTJ2bHBoOTNwMmc1Rkk1V2pNOGZTOFU4Sml3aGZxU3hzMlJUCiAgbXVnNVFFbUJOQ0QzVFo4cXhwOWwycytKNUJlOEdoVEh3NldIeU41bklRS0JnSHNlS0FZTkgwU0tNVEJtRDl0QwogICtETWh3NllweGU0VnNEQkZvRHIxV3hwdDZTbXJaY3k3SmNCTy9qbVhZL3h3c25HeWVoZGJwc014WDAzYzdRU2YKICBBbW9KQ2dPdG0wRlVYYytleWJGemdqTTlkdkIvWmFLUms3THRBMktKakZ0TVBHd0ttTHdqQzQrY0Q4eEw1N0VqCiAgWkVoamZleXVjZDQ4TStKTmJ5TXVFMlpDCiAgLS0tLS1FTkQgUFJJVkFURSBLRVktLS0tLQpyb290Q2E6IGZvbwo=
metadata:
  annotations:
    resource_kind: '*v1.Secret'
  creationTimestamp: null
  name: test
  namespace: gloo-system
`))
		})
	})
})

func mustWriteTestFile(contents string) string {
	tmpFile, err := ioutil.TempFile("", "test-")

	if err != nil {
		log.Fatalf("Failed to create test file: %v", err)
	}

	text := []byte(contents)
	if _, err = tmpFile.Write(text); err != nil {
		log.Fatalf("Failed to write to test file: %v", err)
	}

	if err := tmpFile.Close(); err != nil {
		log.Fatalf("Failed to write to test file: %v", err)
	}

	return tmpFile.Name()
}

// each of these two key pairs were generated as follows:
// openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
//   -keyout privateKey1.key -out privateKey1Cert.crt \
//   -subj "/CN=petstore.example.com"
var privateKey1 = `
-----BEGIN PRIVATE KEY-----
MIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQDKtfsD/lq/htGu
28ivzLzzmZe8fOiqBn2eNUj8QVOlHLnaxTZvrqlkTAnPsDvdHCSTQrZDGI5am1/c
tMKodDbflybgskKvZvDi6BF1w1sbZ3kPGZEt0EnykoiKDvc++XuiNUvMQG02B03v
4iurrLa6TLapB8NWtmRJ3n/Pba500zu+uDF6WFkNTEawzuMMl7jLX5gmkDQeBVvX
12IRY0E1zyq7oTNxsyppxvknhAVKdLYsKRVCtwruC0ACjdwYgq47Nn1S7d7b7M8T
cRkcsHqpy4fUvski1ykfX45kt3QcH8XEpCLPA6B74/1YDAGKfdfBKTsZbeuUt5Q3
v8u2/VJDAgMBAAECggEBALVb/0phZLt6WVtCE9kFKgAf6JugfWCxEe5b6gKWR8mv
W7CZRMzCzZaIWTbRi52VMjv2Ma7x51q1LB0ANDAWWYnNZ+EcW4Embln0GrrrnzVz
a+HQlA0TDzXRWAt8vERBXRWQ7VG+SndOLbJy/XNIwOsI+1tbMK8B29QjFuJ0VOL7
CUoG1/0BXw6sloh9UcoqM4nVcNaqocti4K7d3Z0/T1zJzx6o4o89DqBRnzTj0b9O
ge+sRqGFxEejr2hqMJ0jF5BPYUOyt/4kz1gJcRst0NI9dttwTPWWodu2a3eQT1SF
GmYaJpZQAhknQ6LbFMIdGxLUjt9Ej0D/YRjAvjrs5SECgYEA8o5MJoq8gIMbYFuS
UpR9/qhOh1EaR1D6SEKBpStvVXkRR0JHU77h5gmZZNjzeoGzR5rON5iq2kuKNomW
jt4S6BdEPoR+Kqxu/55uVhmdrSmP/tMXN0XTQa63HscmWUsadBm/E1xAaivYG7W9
GyzsVW+HApm6mTmL2S+YQ1aB1esCgYEA1fJPTiFifjAuhPKOLHQZ6WmgdAWNAsDi
nIoM9BSaxZYNo7o2nes2FIa88hIAtlyCzLMNI8r7xkD3XJEsWoe+VX1gbeTjARpm
W6yLMkYvQsF6Urt4Jw1m2+mCbbh/aNfdaL3HrHevU8mMUaP9iRekXYNh24CaNK+v
t2YRuWsCJwkCgYBgP+Mr8CW5AU2dwPihWFde9D6lJ6O75QBMKEf12PSHAFHA6yYO
r1JIzEpYYFbNqCYSJfXqzeQOV6dy2MoryyfJfWIRRNYj7OTm/mFePS/6hOGlBvLR
dh3MlJ4J0pD/IfRPWeAeuJ6/AsLwy/9Mh1kI1gbHG2WWY+WAu4g6QFupHQKBgQCK
ODWMIH1lUPN86Md5aLik15zV2BA1yy+cOoQL3JPxOvQs5s0KUT9rG3FOYtsa9cF7
ReIjUaw/dRFaOGATTMdmq810sf8GY2vlph93p2g5FI5WjM8fS8U8JiwhfqSxs2RT
mug5QEmBNCD3TZ8qxp9l2s+J5Be8GhTHw6WHyN5nIQKBgHseKAYNH0SKMTBmD9tC
+DMhw6Ypxe4VsDBFoDr1Wxpt6SmrZcy7JcBO/jmXY/xwsnGyehdbpsMxX03c7QSf
AmoJCgOtm0FUXc+eybFzgjM9dvB/ZaKRk7LtA2KJjFtMPGwKmLwjC4+cD8xL57Ej
ZEhjfeyucd48M+JNbyMuE2ZC
-----END PRIVATE KEY-----
`
var privateKey1Cert = `
-----BEGIN CERTIFICATE-----
MIICvDCCAaQCCQDro6ZXybhlYDANBgkqhkiG9w0BAQsFADAgMR4wHAYDVQQDDBVw
ZXRzdG9yZTEuZXhhbXBsZS5jb20wHhcNMTkwNDA1MTkyODQ2WhcNMjAwNDA0MTky
ODQ2WjAgMR4wHAYDVQQDDBVwZXRzdG9yZTEuZXhhbXBsZS5jb20wggEiMA0GCSqG
SIb3DQEBAQUAA4IBDwAwggEKAoIBAQDKtfsD/lq/htGu28ivzLzzmZe8fOiqBn2e
NUj8QVOlHLnaxTZvrqlkTAnPsDvdHCSTQrZDGI5am1/ctMKodDbflybgskKvZvDi
6BF1w1sbZ3kPGZEt0EnykoiKDvc++XuiNUvMQG02B03v4iurrLa6TLapB8NWtmRJ
3n/Pba500zu+uDF6WFkNTEawzuMMl7jLX5gmkDQeBVvX12IRY0E1zyq7oTNxsypp
xvknhAVKdLYsKRVCtwruC0ACjdwYgq47Nn1S7d7b7M8TcRkcsHqpy4fUvski1ykf
X45kt3QcH8XEpCLPA6B74/1YDAGKfdfBKTsZbeuUt5Q3v8u2/VJDAgMBAAEwDQYJ
KoZIhvcNAQELBQADggEBAL8m5TjFEb58MEXKGdbyGZEdS0FpNI+fYBYyxkpU5/z3
06hV2ajisgvHGyGun/HLBDXtWnbNWKpSjiJiS9Kpkv6X73hba6Q3p3prjgdXkpSU
ONozwlMM1SM0dj/O5VULkcW4uhSQJEyIRRPiA8fslqWuIlr5KWWPbdIkDex/9Ddf
oC7D1exclZNVDVmJzYFSxb1js/rSsln11VJ7uyozpk23lrAVGIrtg5Xr4vxqUZHU
TOeFSVH6LMC5j/Fff+bEBhbPxJAI0P7VXaphYh/dMyAEq+xRxm6ssuccgCyvttmz
+6sUivvxaDhUCAzAoLSa5Xgn5eNdsePz6PQ5Vy/Yidg=
-----END CERTIFICATE-----
`

var privateKey2 = `
-----BEGIN PRIVATE KEY-----
MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQC8Qb58Ycam8i9l
fNNHNrXTp5gxb8qD6obfQ1oKbbp+hyzZ+JOsZSFHkqZ1kNAV3ZJqEJItR5zqm4r9
YprZVDEHkby+LlstSMcUp+KxPXPFnNSZEi3WdAByScf2d08GDpalAPlZRJorIvUA
gwO2svGfRuoqwb1pR4rLKpDi+TMt7R0dLwsU02ehH0d0tV0Cf9v2zkcFhXjeEFjp
KStbd3x9mxePoYMwkHiEbDGkwavdSp5deQ3DdSYEiwLwTaAeKgHp8XhxHYFmiO28
n4Sn7W/2Ln9oUExjkP12bXs/zpVovVclnrzbizWU9aQsmzv+bCr1h5hc4kO06X7V
AfIlxgmRAgMBAAECggEAa34qs0DtOjQ9Zfihdx9BMWqX11qayzol6uO9TQkcnNS4
gnmScLSKDSEqlSSr/GA5EwEFRn+GlNtdwJMGEiQlnsnTeNBlVXUh36SBJ20MZwDG
z+R2ceZovtlsKUo0wCOiVvM4bYVjdlAOj00/2JlSp/zJBsL7UVr9YEac1k0usQCh
XLOgak13/X8lmwYg/G1V2xFaLIIddAECmA0NnzJjZ1QNiBd0EfZakyAWFrTDCJIx
+KZF3Pbk4uS5GJAWvoWSTU/x5arIOhvu6c1np8643vu3W5LbeVyQGjM2n8xXTMN2
3IJFWjkzViY7ThqxWzzpX8YjLMZHArMea7SZbpr9pQKBgQDk1UCE7SQaaWtj9uk5
KLdltCyB7+TSUQYUAj5n/24yDHTdd8iTE85wOSAFqG4JXpnxz4gpf7qFd0uNFcke
TNB+YJeRl4kL21Lz7s04FlU/gsHW1izE6kjQlyFD9dW1z5OF+Jjg0BCtdHwOfKCZ
fUun0vhJgJKLPo1jAWgGawic4wKBgQDSm0pUnMcIBTNu2Y8CEo/SnnHRr8vN66fp
Luo7VG3POrDzhsTNtKlDlt3NoMmJIfpwC9JugejDsGEnGp4D8BBexftiTMItxlgS
UwBdISilzgRPJi2iFYSq8remQ80/SKN8/V+2Es75ZGcomOdzAdxPMV4C911iN47M
3jftov+d+wKBgC51xaW3aA8cvDsNlIiQZbv2etre0/yHis5hLj57M+phcRDOEyEZ
cl6CmqfLbJvmYycfVavnTP1wHRzGAZFvUx11ixB6Tc7kdtEj+PKcRi6g4640yd4p
GyOOq6har0s8m90lfhSW6evtrIpcb1b6g3PNd6+ktRwkVRx22qIC9Tq1AoGAeLXv
HZ4aadtpRmDGGd7/ti2AeTn0a1tliz6LnGPg6ITwRTR6epjQ51+CU8iTmtjxzOTJ
wPMOsZLXrG0SIpmnGFsLoaTzKv9jHWWbcMV/ocD6MU9lmmARAVZKsq5r5pjAs/QZ
tqcDIGhOxDMXfZCUcIOQKc0UJiZH396CWd8x+Z8CgYEAscPWpibUwa6XL1rBFI2T
NW9D/dcnYIw0b9rRjbFh6JMt0XOPcVHdKclXIJXVPMm7lLBjeENdf0F4+JTAbksE
LejlAwmj9Wi/nRSZprg56evz1CpLiy9Ss0cZh/vnR29FLLGV6sfBBCFsZRCJxnYW
OMmQCizD5B5Gw099ZBN+ErM=
-----END PRIVATE KEY-----
`
var privateKey2Cert = `
-----BEGIN CERTIFICATE-----
MIICvDCCAaQCCQCg3+BPM3PJ9zANBgkqhkiG9w0BAQsFADAgMR4wHAYDVQQDDBVw
ZXRzdG9yZTIuZXhhbXBsZS5jb20wHhcNMTkwNDA1MTkyOTAyWhcNMjAwNDA0MTky
OTAyWjAgMR4wHAYDVQQDDBVwZXRzdG9yZTIuZXhhbXBsZS5jb20wggEiMA0GCSqG
SIb3DQEBAQUAA4IBDwAwggEKAoIBAQC8Qb58Ycam8i9lfNNHNrXTp5gxb8qD6obf
Q1oKbbp+hyzZ+JOsZSFHkqZ1kNAV3ZJqEJItR5zqm4r9YprZVDEHkby+LlstSMcU
p+KxPXPFnNSZEi3WdAByScf2d08GDpalAPlZRJorIvUAgwO2svGfRuoqwb1pR4rL
KpDi+TMt7R0dLwsU02ehH0d0tV0Cf9v2zkcFhXjeEFjpKStbd3x9mxePoYMwkHiE
bDGkwavdSp5deQ3DdSYEiwLwTaAeKgHp8XhxHYFmiO28n4Sn7W/2Ln9oUExjkP12
bXs/zpVovVclnrzbizWU9aQsmzv+bCr1h5hc4kO06X7VAfIlxgmRAgMBAAEwDQYJ
KoZIhvcNAQELBQADggEBAHO8zf32PEY/kgsL91Sz6vmQ/ji8im52zZxvvVsd2529
Ebhfc2pRBl6piHsP37S/xzDwdXPqY97uaKa79ePr8WykDVDNeQUbqJ+BlLl44RgL
N0UVWFROcq0IKAHwQpNoiknIRRNqe9GyVJ5mqSx+DynwWZV50fx5LHo/NCkgTBi6
BRvWlaKeAv7oVJbkyL0EgspWMIM9/OV9qVufQV0KKJC0qh/kjhc3B0SQJZ+5v+d6
kQsXj5o8QG0qlHZ+Ip3EAx55T+9M7ny61kQyfERdmqMfXJ2pcVYIYfiBfsKxwsFq
DglLoV+6OsTclI7yYTcERu1uay53HFe6DFMtejYAbSo=
-----END CERTIFICATE-----
`
