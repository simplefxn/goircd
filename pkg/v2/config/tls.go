package config

import (
	"bufio"
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"time"
)

func IfKeyPairExists(cfg *Certificate) error {

	_, err := os.Stat(cfg.Certificate)

	if err == nil {
		return fmt.Errorf("file %s exists", cfg.Certificate)
	}

	_, err = os.Stat(cfg.Key)

	if err == nil {
		return fmt.Errorf("file %s exists", cfg.Key)

	}
	return nil
}

func SaveCertificateToFile(fileName string, caBytes []byte) error {
	// pem encode
	caPEM := new(bytes.Buffer)
	err := pem.Encode(caPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caBytes,
	})
	if err != nil {
		return err
	}

	if err := SaveToFile(fileName, caPEM); err != nil {
		return err
	}

	return nil
}

func SaveKeyToFile(fileName string, caPrivKey *rsa.PrivateKey) error {
	// pem encode
	caPrivKeyPEM := new(bytes.Buffer)
	err := pem.Encode(caPrivKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(caPrivKey),
	})

	if err != nil {
		return err
	}

	if err := SaveToFile(fileName, caPrivKeyPEM); err != nil {
		return err
	}

	return nil
}

func SaveToFile(fileName string, buf *bytes.Buffer) error {
	// open output file
	fo, err := os.Create(fileName)
	if err != nil {
		return err
	}

	fileWriter := bufio.NewWriter(fo)

	// close fo on exit dont check on error on close
	defer func() {
		if err := fo.Close(); err != nil {
			return
		}
	}()

	if _, err := buf.WriteTo(fileWriter); err != nil {
		return err
	}

	if err := fileWriter.Flush(); err != nil {
		return err
	}
	return nil
}

func CreateCertificate(certCfg *Certificate) *x509.Certificate {
	return &x509.Certificate{
		SerialNumber: big.NewInt(certCfg.Attributes.Serial),
		Subject: pkix.Name{
			Organization:       []string{certCfg.Attributes.Organization},
			OrganizationalUnit: []string{certCfg.Attributes.OrganizationalUnit},
			CommonName:         certCfg.Attributes.CommonName,
			Country:            []string{certCfg.Attributes.Country},
			Province:           []string{certCfg.Attributes.State},
			Locality:           []string{certCfg.Attributes.Locality},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(0, 12, 0),
		BasicConstraintsValid: true,
	}
}
