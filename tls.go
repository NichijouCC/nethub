package nethub

import (
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"log"
	"os"
	"time"
)

type Options struct {
	caFile   string
	password string
}

type TlsOptions func(options *Options)

func WithCaFile(caFile string) TlsOptions {
	return func(options *Options) {
		options.caFile = caFile
	}
}
func WithPassword(password string) TlsOptions {
	return func(options *Options) {
		options.password = password
	}
}

func NewTlsConfig(certFile, keyFile string, opt ...TlsOptions) (*tls.Config, error) {
	var opts Options
	for _, o := range opt {
		o(&opts)
	}
	certByte, err := os.ReadFile(certFile)
	if err != nil {
		return nil, err
	}
	keyByte, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, err
	}

	keyPem, rest := pem.Decode(keyByte)
	if len(rest) > 0 {
		return nil, errors.New("Decode key failed!")
	}
	if x509.IsEncryptedPEMBlock(keyPem) {
		if opts.password == "" {
			return nil, errors.New("keyFile need passWord param")
		}
		keyDePemByte, err := x509.DecryptPEMBlock(keyPem, []byte(opts.password))
		if err != nil {
			return nil, err
		}
		key, err := x509.ParsePKCS1PrivateKey(keyDePemByte)
		if err != nil {
			return nil, err
		}
		keyByte = pem.EncodeToMemory(&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(key),
		})
	}
	cert, err := tls.X509KeyPair(certByte, keyByte)
	if err != nil {
		return nil, err
	}
	config := &tls.Config{
		Certificates: []tls.Certificate{cert},
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		},
	}
	now := time.Now()
	config.Time = func() time.Time { return now }
	config.Rand = rand.Reader
	if opts.caFile != "" {
		caByte, err := os.ReadFile(opts.caFile)
		if err != nil {
			return nil, err
		}
		certPool := x509.NewCertPool()
		if !certPool.AppendCertsFromPEM(caByte) {
			log.Fatalf("Can't parse CA certificate")
		}
		config.ClientCAs = certPool
		config.ClientAuth = tls.RequireAndVerifyClientCert
	}
	return config, nil
}
