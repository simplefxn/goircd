package config

import (
	"crypto/rsa"
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
	CARoot Certificate   `yaml:"certificate_authority" json:"certificate_authority"`
	Certs  []Certificate `yaml:"certificates" json:"certificates"`
}

type Certificate struct {
	Type        string     `yaml:"type" json:"type"`
	Create      bool       `yaml:"create" json:"create"`
	Certificate string     `yaml:"cert" json:"cert"`
	Key         string     `yaml:"key" json:"key"`
	Attributes  Attributes `yaml:"attributes" json:"attributes"`
}

type Attributes struct {
	Locality           string `yaml:"locality" json:"locality"`
	State              string `yaml:"state" json:"state"`
	Country            string `yaml:"country" json:"country"`
	Organization       string `yaml:"organization" json:"organization"`
	OrganizationalUnit string `yaml:"organizational_unit" json:"organizational_unit"`
	Serial             int64  `yaml:"serial" json:"serial"`
	CommonName         string `yaml:"common_name" json:"common_name"`
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
