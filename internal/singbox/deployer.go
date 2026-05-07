package singbox

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/curve25519"
)

// ServerDeployOptions describes a sb.sh-like server deployment preparation.
type ServerDeployOptions struct {
	ExternalHost       string
	Hostname           string
	UUID               string
	Password           string
	RealitySNI         string
	RealityPrivateKey  string
	RealityPublicKey   string
	RealityShortID     string
	WebSocketPath      string
	CertificatePath    string
	PrivateKeyPath     string
	VlessRealityPort   int
	VmessWebSocketPort int
	Hysteria2Port      int
	TUICPort           int
	AnyTLSPort         int
	ConfigName         string
}

// ServerDeployResult contains the saved server config and client links.
type ServerDeployResult struct {
	Config            *SingboxConfig    `json:"config"`
	ConfigPath        string            `json:"config_path"`
	Links             map[string]string `json:"links"`
	UUID              string            `json:"uuid"`
	Password          string            `json:"password"`
	RealityPrivateKey string            `json:"reality_private_key"`
	RealityPublicKey  string            `json:"reality_public_key"`
	RealityShortID    string            `json:"reality_short_id"`
	CertificatePath   string            `json:"certificate_path"`
	PrivateKeyPath    string            `json:"private_key_path"`
}

type ServerDeployer struct {
	paths ConfigPaths
}

func NewServerDeployer() *ServerDeployer {
	env := DetectEnvironment()
	return &ServerDeployer{paths: GetConfigPaths(env)}
}

func (d *ServerDeployer) Prepare(options ServerDeployOptions) (*ServerDeployResult, error) {
	opts, configName, err := d.normalizeOptions(options)
	if err != nil {
		return nil, err
	}

	config, err := BuildServerConfig(opts)
	if err != nil {
		return nil, err
	}
	if err := ensureSelfSignedCertificate(opts); err != nil {
		return nil, err
	}
	links, err := GenerateShareLinks(opts)
	if err != nil {
		return nil, err
	}

	generator := NewConfigGenerator()
	if err := generator.SaveConfig(config, configName); err != nil {
		return nil, err
	}

	return &ServerDeployResult{
		Config:            config,
		ConfigPath:        filepath.Join(d.paths.ConfigDir, configName),
		Links:             links,
		UUID:              opts.UUID,
		Password:          opts.Password,
		RealityPrivateKey: opts.RealityPrivateKey,
		RealityPublicKey:  opts.RealityPublicKey,
		RealityShortID:    opts.RealityShortID,
		CertificatePath:   opts.CertificatePath,
		PrivateKeyPath:    opts.PrivateKeyPath,
	}, nil
}

func (d *ServerDeployer) normalizeOptions(options ServerDeployOptions) (ServerConfigOptions, string, error) {
	uuid := strings.TrimSpace(options.UUID)
	if uuid == "" {
		var err error
		uuid, err = generateUUID()
		if err != nil {
			return ServerConfigOptions{}, "", fmt.Errorf("generate uuid: %w", err)
		}
	}

	password := strings.TrimSpace(options.Password)
	if password == "" {
		var err error
		password, err = generatePassword(18)
		if err != nil {
			return ServerConfigOptions{}, "", fmt.Errorf("generate password: %w", err)
		}
	}

	privateKey := strings.TrimSpace(options.RealityPrivateKey)
	publicKey := strings.TrimSpace(options.RealityPublicKey)
	if privateKey == "" || publicKey == "" {
		var err error
		privateKey, publicKey, err = GenerateRealityKeyPair()
		if err != nil {
			return ServerConfigOptions{}, "", fmt.Errorf("generate reality keypair: %w", err)
		}
	}

	shortID := strings.TrimSpace(options.RealityShortID)
	if shortID == "" {
		generatedShortID, err := GenerateShortID()
		if err != nil {
			return ServerConfigOptions{}, "", fmt.Errorf("generate short id: %w", err)
		}
		shortID = generatedShortID
	}

	certPath := strings.TrimSpace(options.CertificatePath)
	if certPath == "" {
		certPath = filepath.Join(d.paths.ConfigDir, "cert.pem")
	}
	keyPath := strings.TrimSpace(options.PrivateKeyPath)
	if keyPath == "" {
		keyPath = filepath.Join(d.paths.ConfigDir, "private.key")
	}

	configName := strings.TrimSpace(options.ConfigName)
	if configName == "" {
		configName = "sb.json"
	}
	if filepath.Base(configName) != configName {
		return ServerConfigOptions{}, "", fmt.Errorf("config name must not contain path separators")
	}

	opts := ServerConfigOptions{
		ExternalHost:       strings.TrimSpace(options.ExternalHost),
		Hostname:           strings.TrimSpace(options.Hostname),
		UUID:               uuid,
		Password:           password,
		RealitySNI:         strings.TrimSpace(options.RealitySNI),
		RealityPrivateKey:  privateKey,
		RealityPublicKey:   publicKey,
		RealityShortID:     shortID,
		WebSocketPath:      strings.TrimSpace(options.WebSocketPath),
		CertificatePath:    certPath,
		PrivateKeyPath:     keyPath,
		VlessRealityPort:   defaultPort(options.VlessRealityPort, 10000),
		VmessWebSocketPort: defaultPort(options.VmessWebSocketPort, 10001),
		Hysteria2Port:      defaultPort(options.Hysteria2Port, 10002),
		TUICPort:           defaultPort(options.TUICPort, 10003),
		AnyTLSPort:         defaultPort(options.AnyTLSPort, 10004),
	}
	return opts, configName, nil
}

func defaultPort(value, fallback int) int {
	if value == 0 {
		return fallback
	}
	return value
}

func GenerateRealityKeyPair() (string, string, error) {
	privateKey := make([]byte, curve25519.ScalarSize)
	if _, err := rand.Read(privateKey); err != nil {
		return "", "", err
	}
	privateKey[0] &= 248
	privateKey[31] &= 127
	privateKey[31] |= 64

	publicKey, err := curve25519.X25519(privateKey, curve25519.Basepoint)
	if err != nil {
		return "", "", err
	}

	return base64.RawURLEncoding.EncodeToString(privateKey), base64.RawURLEncoding.EncodeToString(publicKey), nil
}

func ensureSelfSignedCertificate(opts ServerConfigOptions) error {
	if fileExists(opts.CertificatePath) && fileExists(opts.PrivateKeyPath) {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(opts.CertificatePath), 0755); err != nil {
		return fmt.Errorf("create certificate directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(opts.PrivateKeyPath), 0755); err != nil {
		return fmt.Errorf("create private key directory: %w", err)
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("generate certificate key: %w", err)
	}

	cert, err := createSelfSignedCertificate(privateKey, opts)
	if err != nil {
		return err
	}
	certFile, err := os.Create(opts.CertificatePath)
	if err != nil {
		return fmt.Errorf("create certificate file: %w", err)
	}
	defer certFile.Close()
	if err := pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: cert}); err != nil {
		return fmt.Errorf("write certificate: %w", err)
	}

	keyFile, err := os.OpenFile(opts.PrivateKeyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("create private key file: %w", err)
	}
	defer keyFile.Close()
	keyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return fmt.Errorf("marshal private key: %w", err)
	}
	if err := pem.Encode(keyFile, &pem.Block{Type: "PRIVATE KEY", Bytes: keyBytes}); err != nil {
		return fmt.Errorf("write private key: %w", err)
	}

	return nil
}

func createSelfSignedCertificate(privateKey *rsa.PrivateKey, opts ServerConfigOptions) ([]byte, error) {
	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return nil, fmt.Errorf("generate serial number: %w", err)
	}

	commonName := strings.TrimSpace(opts.Hostname)
	if commonName == "" {
		commonName = "www.bing.com"
	}
	template := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   commonName,
			Organization: []string{"Sing-box"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().AddDate(100, 0, 0),
		KeyUsage:  x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
	}
	addCertificateSANs(&template, opts, commonName)

	cert, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, fmt.Errorf("create certificate: %w", err)
	}
	return cert, nil
}

func addCertificateSANs(template *x509.Certificate, opts ServerConfigOptions, commonName string) {
	for _, value := range []string{commonName, opts.Hostname, opts.ExternalHost, opts.RealitySNI} {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if ip := net.ParseIP(value); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
			continue
		}
		template.DNSNames = append(template.DNSNames, value)
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
