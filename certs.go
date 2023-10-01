package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"unsafe"

	"github.com/simplefxn/goircd/pkg/v2/config"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"
)

func CmdCAGenerate() *cli.Command {
	return &cli.Command{

		Name:  "ca",
		Usage: "generate certificates",
		Subcommands: []*cli.Command{
			{
				Name:  "generate",
				Usage: "generate certificates",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:        "caConfig",
						Usage:       "certificate authority config file",
						Value:       "",
						Destination: &config.SSLConfigFile,
					},
				},
				Action: func(cCtx *cli.Context) error {
					if config.SSLConfigFile != "" {
						var provisionerCfg config.CAConfig

						yamlCfg, err := os.ReadFile(config.SSLConfigFile)
						if err != nil {
							return err
						}
						err = yaml.Unmarshal(yamlCfg, &provisionerCfg)
						if err != nil {
							return err
						}

						caRootCfg := provisionerCfg.CARoot
						caCfg := *(*config.Certificate)(unsafe.Pointer(&caRootCfg))

						if err = config.IfKeyPairExists(&caCfg); err == nil && caCfg.Create {
							// set up our CA certificate
							ca := config.CreateCertificate(&caCfg)
							ca.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth}
							ca.IsCA = true
							ca.KeyUsage = x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign

							// create our private and public key
							caPrivKey, err := rsa.GenerateKey(rand.Reader, 4096)
							if err != nil {
								return err
							}

							// create the CA
							caBytes, err := x509.CreateCertificate(rand.Reader, ca, ca, &caPrivKey.PublicKey, caPrivKey)
							if err != nil {
								return err
							}

							if saveCertErr := config.SaveCertificateToFile(caCfg.Certificate, caBytes); saveCertErr != nil {
								return saveCertErr
							}

							if saveKeyErr := config.SaveKeyToFile(caCfg.Key, caPrivKey); saveKeyErr != nil {
								return saveKeyErr
							}
						}

						// Read CA again
						err = config.IfKeyPairExists(&caCfg)
						if err == nil {
							return fmt.Errorf("cannot find root ca cert/key")
						}

						// Load CA Cert/Key for signing
						ca, caPrivateKey := caCfg.Loadx509KeyPair()

						for _, certCfg := range provisionerCfg.Certs {
							if err = config.IfKeyPairExists(&certCfg); err == nil {
								cert := config.CreateCertificate(&certCfg)
								if certCfg.Type == "server" {
									cert.IPAddresses = []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback}
									cert.DNSNames = []string{"localhost"}
									cert.SubjectKeyId = []byte{1, 2, 3, 4, 6}
									cert.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageIPSECEndSystem}
									cert.KeyUsage = x509.KeyUsageDigitalSignature
								}

								certPrivKey, genErr := rsa.GenerateKey(rand.Reader, 4096)
								if genErr != nil {
									return genErr
								}

								certBytes, createErr := x509.CreateCertificate(rand.Reader, cert, ca, &certPrivKey.PublicKey, caPrivateKey)
								if createErr != nil {
									return createErr
								}

								if saveCertErr := config.SaveCertificateToFile(certCfg.Certificate, certBytes); saveCertErr != nil {
									return saveCertErr
								}

								if configErr := config.SaveKeyToFile(certCfg.Key, certPrivKey); configErr != nil {
									return configErr
								}
							}
						}
					} else {
						return fmt.Errorf("cannot generate certificates without config file")
					}
					return nil
				},
			},
		},
	}
}
