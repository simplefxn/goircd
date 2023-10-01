package config

import (
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
)

var (
	SSLConfigFile string
	config        Bootstrap
)

type Bootstrap struct {
	Hostname string `yaml:"hostname"`
	Bind     string `yaml:"bind"`
	Motd     string `yaml:"motd"`
	SSLKey   string `yaml:"sslKey"`
	SSLCert  string `yaml:"sslCert"`
	SSLCA    string `yaml:"sslCA"`
}

type CAConfig struct {
	Certs  []Certificate `yaml:"certificates" json:"certificates"`
	CARoot Certificate   `yaml:"certificate_authority" json:"certificate_authority"`
}

type Certificate struct {
	Type        string     `yaml:"type" json:"type"`
	Certificate string     `yaml:"cert" json:"cert"`
	Key         string     `yaml:"key" json:"key"`
	Attributes  Attributes `yaml:"attributes" json:"attributes"`
	Create      bool       `yaml:"create" json:"create"`
}

type Attributes struct {
	Locality           string `yaml:"locality" json:"locality"`
	State              string `yaml:"state" json:"state"`
	Country            string `yaml:"country" json:"country"`
	Organization       string `yaml:"organization" json:"organization"`
	OrganizationalUnit string `yaml:"organizational_unit" json:"organizational_unit"`
	CommonName         string `yaml:"common_name" json:"common_name"`
	Serial             int64  `yaml:"serial" json:"serial"`
}

func Get() *Bootstrap {
	return &config
}

func (c *Certificate) Loadx509KeyPair() (*x509.Certificate, *rsa.PrivateKey) {
	cf, e := os.ReadFile(c.Certificate)
	if e != nil {
		fmt.Println("cfload:", e.Error())
		os.Exit(1)
	}

	kf, e := os.ReadFile(c.Key)
	if e != nil {
		fmt.Println("kfload:", e.Error())
		os.Exit(1)
	}

	cpb, _ := pem.Decode(cf)

	kpb, _ := pem.Decode(kf)

	crt, e := x509.ParseCertificate(cpb.Bytes)

	if e != nil {
		fmt.Println("parsex509:", e.Error())
		os.Exit(1)
	}

	key, e := x509.ParsePKCS1PrivateKey(kpb.Bytes)
	if e != nil {
		fmt.Println("parsekey:", e.Error())
		os.Exit(1)
	}

	return crt, key
}

func (b *Bootstrap) GetServerConfig() *tls.Config {
	cert, err := tls.LoadX509KeyPair(b.SSLCert, b.SSLKey)
	if err != nil {
		return nil
	}

	bytes, err := os.ReadFile(b.SSLCA)
	if err != nil {
		return nil
	}

	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM(bytes)

	config := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientCAs:    certPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS12,
	}

	return config
}
