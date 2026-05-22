// Package certmgr encapsulates the certificate setup business from
// sb.sh:241-317 (inscertificate / ymzs / zqzs).
//
// Two responsibilities:
//
//  1. Generate a self-signed ECC certificate at /etc/s-box/cert.pem +
//     /etc/s-box/private.key (mirrors sb.sh:283-285 zqzs block).
//
//  2. Detect whether the legacy Acme-yg flow already produced a real domain
//     certificate at /root/ygkkkca/{cert.crt,private.key,ca.log}. Detection
//     is read-only — actually running acme.sh is out of scope (sb.sh:300
//     shells out to an external script we deliberately don't depend on).
//
// The package emits OperationPlans through the systemops装饰器体系 so the
// usual audit + progress channels apply.
package certmgr

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"miaomiaowu/internal/systemops"
)

// File paths managed by sb.sh.
const (
	SBoxDir        = "/etc/s-box"
	SelfSignedCert = SBoxDir + "/cert.pem"
	SelfSignedKey  = SBoxDir + "/private.key"
	AcmeDir        = "/root/ygkkkca"
	AcmeCertPath   = AcmeDir + "/cert.crt"
	AcmeKeyPath    = AcmeDir + "/private.key"
	AcmeLogPath    = AcmeDir + "/ca.log"
	DefaultCertCN  = "www.bing.com"
	defaultDays    = 36500
	certBackupExt  = ".bak"

	// PlanNameSelfSigned is the audit-table identifier for self-sign runs.
	PlanNameSelfSigned = "certmgr.self_sign"

	stepGenerateID = "generate-self-signed"
)

// validCNPattern restricts the CN to RFC 1035-compatible host labels. The CN
// is interpolated into the certificate Subject directly, so anything else is
// rejected.
var validCNPattern = regexp.MustCompile(`^[a-zA-Z0-9.\-]+$`)

// SelfSignRequest controls the self-signed certificate generation.
type SelfSignRequest struct {
	// CommonName is the X.509 subject CN. Empty falls back to DefaultCertCN.
	CommonName string

	// Days controls cert validity. Zero falls back to defaultDays (sb.sh: 36500
	// — a century, matching the upstream script's "set and forget" stance).
	Days int

	// CertPath / KeyPath override the on-disk destinations. Empty falls back
	// to SelfSignedCert / SelfSignedKey.
	CertPath string
	KeyPath  string
}

// ValidateRequest is the single chokepoint user-supplied CN strings must pass
// before reaching the certificate Subject or any filesystem write.
func ValidateRequest(req SelfSignRequest) error {
	if req.Days < 0 {
		return fmt.Errorf("days must be non-negative, got %d", req.Days)
	}
	if req.Days > 36525 { // 100 years
		return fmt.Errorf("days must be <= 36525 (≈100 years), got %d", req.Days)
	}
	cn := strings.TrimSpace(req.CommonName)
	if cn == "" {
		return nil
	}
	if len(cn) > 253 {
		return fmt.Errorf("common name too long: %d bytes (max 253)", len(cn))
	}
	if !validCNPattern.MatchString(cn) {
		return fmt.Errorf("common name %q contains invalid characters (allowed: a-zA-Z0-9.-)", cn)
	}
	return nil
}

// BuildSelfSignPlan composes a single-step OperationPlan that delegates the
// actual openssl-equivalent work to a custom StepExecutor.
//
// Atomicity is provided by the executor: it writes both files via a
// tmp+rename dance so a half-written certificate cannot land on disk. Single-
// step plan ⇒ no RollbackOnFailure (mirrors the cron package convention).
func BuildSelfSignPlan(req SelfSignRequest) (systemops.OperationPlan, error) {
	if err := ValidateRequest(req); err != nil {
		return systemops.OperationPlan{}, err
	}

	cn := req.CommonName
	if strings.TrimSpace(cn) == "" {
		cn = DefaultCertCN
	}
	days := req.Days
	if days == 0 {
		days = defaultDays
	}
	certPath := req.CertPath
	if certPath == "" {
		certPath = SelfSignedCert
	}
	keyPath := req.KeyPath
	if keyPath == "" {
		keyPath = SelfSignedKey
	}

	step := systemops.OperationStep{
		ID:          stepGenerateID,
		Title:       fmt.Sprintf("生成 ECC 自签证书 (CN=%s, %d 天)", cn, days),
		Description: "openssl ecparam -genkey prime256v1 + openssl req -new -x509",
		Kind:        systemops.StepKindFile,
		Risk:        systemops.RiskLevelMedium,
		Target:      certPath,
		Command:     "certmgr-self-sign",
		Metadata: map[string]string{
			"cn":        cn,
			"days":      fmt.Sprintf("%d", days),
			"cert_path": certPath,
			"key_path":  keyPath,
		},
	}

	plan := systemops.OperationPlan{
		Name:              PlanNameSelfSigned,
		Description:       "生成 ECC 自签证书并落地到 /etc/s-box/",
		RollbackOnFailure: false,
		Steps:             []systemops.OperationStep{step},
	}
	if err := plan.Validate(); err != nil {
		return systemops.OperationPlan{}, fmt.Errorf("validate plan: %w", err)
	}
	return plan, nil
}

// AcmeStatus describes what the package found under /root/ygkkkca.
type AcmeStatus struct {
	Present  bool   `json:"present"`
	Domain   string `json:"domain,omitempty"`    // ca.log contents (typically the registered domain)
	CertPath string `json:"cert_path,omitempty"` // populated when Present
	KeyPath  string `json:"key_path,omitempty"`  // populated when Present
	CertSize int64  `json:"cert_size,omitempty"` // bytes
	KeySize  int64  `json:"key_size,omitempty"`  // bytes
}

// DetectAcme returns whether the legacy Acme-yg flow already produced a usable
// domain certificate. Mirrors the sb.sh:288 `if [[ -f ... && -s ... ]]` test
// but exposes the diagnostic facets to the API consumer.
//
// The function never falls back to running acme.sh — that side effect lives
// outside the package boundary.
func DetectAcme() (AcmeStatus, error) {
	return detectAcmeIn(AcmeDir)
}

func detectAcmeIn(dir string) (AcmeStatus, error) {
	certPath := filepath.Join(dir, "cert.crt")
	keyPath := filepath.Join(dir, "private.key")
	logPath := filepath.Join(dir, "ca.log")

	cert, certErr := os.Stat(certPath)
	key, keyErr := os.Stat(keyPath)
	if certErr != nil || keyErr != nil || cert.Size() == 0 || key.Size() == 0 {
		return AcmeStatus{Present: false}, nil
	}

	status := AcmeStatus{
		Present:  true,
		CertPath: certPath,
		KeyPath:  keyPath,
		CertSize: cert.Size(),
		KeySize:  key.Size(),
	}
	if data, err := os.ReadFile(logPath); err == nil {
		status.Domain = strings.TrimSpace(string(data))
	}
	return status, nil
}

// Executor wires the certmgr-self-sign virtual command to the Go crypto
// stdlib. Stubs (NowFunc / RandReader) make the executor unit-testable.
type Executor struct {
	NowFunc    func() time.Time
	RandReader func() (io.Reader, error)
}

// NewExecutor returns a production-wired Executor.
func NewExecutor() *Executor {
	return &Executor{
		NowFunc:    time.Now,
		RandReader: func() (io.Reader, error) { return rand.Reader, nil },
	}
}

// ExecuteStep performs the certificate + key generation requested by the step.
func (e *Executor) ExecuteStep(_ context.Context, step systemops.OperationStep) error {
	if step.Command != "certmgr-self-sign" {
		return fmt.Errorf("certmgr: unsupported command %q", step.Command)
	}
	if step.Metadata == nil {
		return errors.New("certmgr: missing metadata")
	}

	cn := step.Metadata["cn"]
	certPath := step.Metadata["cert_path"]
	keyPath := step.Metadata["key_path"]
	if certPath == "" || keyPath == "" {
		return errors.New("certmgr: cert_path or key_path missing in metadata")
	}

	days := 0
	if d := step.Metadata["days"]; d != "" {
		if _, err := fmt.Sscanf(d, "%d", &days); err != nil {
			return fmt.Errorf("certmgr: parse days: %w", err)
		}
	}
	if days <= 0 {
		days = defaultDays
	}

	now := time.Now()
	if e.NowFunc != nil {
		now = e.NowFunc()
	}
	randSource, err := e.randSource()
	if err != nil {
		return fmt.Errorf("certmgr: random source: %w", err)
	}

	priv, err := ecdsa.GenerateKey(elliptic.P256(), randSource)
	if err != nil {
		return fmt.Errorf("certmgr: generate ECC key: %w", err)
	}

	serial, err := generateSerial(randSource)
	if err != nil {
		return fmt.Errorf("certmgr: serial: %w", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             now.Add(-1 * time.Hour),
		NotAfter:              now.AddDate(0, 0, days),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}

	derBytes, err := x509.CreateCertificate(randSource, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		return fmt.Errorf("certmgr: create certificate: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})

	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return fmt.Errorf("certmgr: marshal key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	if err := atomicWrite(certPath, certPEM, 0o644); err != nil {
		return fmt.Errorf("certmgr: write cert: %w", err)
	}
	if err := atomicWrite(keyPath, keyPEM, 0o600); err != nil {
		_ = os.Remove(certPath)
		return fmt.Errorf("certmgr: write key: %w", err)
	}
	return nil
}

func (e *Executor) randSource() (io.Reader, error) {
	if e.RandReader == nil {
		return rand.Reader, nil
	}
	return e.RandReader()
}

func generateSerial(r io.Reader) (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	return rand.Int(r, limit)
}

// atomicWrite writes data to a sibling tmp file then renames into place so a
// crash mid-write cannot leave a corrupt half-cert on disk.
func atomicWrite(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("ensure dir %s: %w", dir, err)
	}
	tmp := path + certBackupExt + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
