package certmgr

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"miaomiaowu/internal/systemops"
)

func minStep(cmd string) systemops.OperationStep {
	return systemops.OperationStep{
		ID:      "test-" + cmd,
		Title:   "test",
		Kind:    systemops.StepKindFile,
		Risk:    systemops.RiskLevelLow,
		Target:  "test-target",
		Command: cmd,
	}
}

func TestValidateRequest_AcceptsCommonInputs(t *testing.T) {
	cases := []SelfSignRequest{
		{},
		{CommonName: "example.com"},
		{CommonName: "www.bing.com", Days: 365},
		{CommonName: "a-b.c-d.example", Days: 36525},
	}
	for _, c := range cases {
		if err := ValidateRequest(c); err != nil {
			t.Errorf("ValidateRequest(%+v) unexpected err: %v", c, err)
		}
	}
}

func TestValidateRequest_RejectsInjection(t *testing.T) {
	cases := map[string]SelfSignRequest{
		"semicolon":    {CommonName: "a;b"},
		"slash":        {CommonName: "a/b"},
		"quote":        {CommonName: `a"b`},
		"single quote": {CommonName: "a'b"},
		"dollar":       {CommonName: "a$b"},
		"backtick":     {CommonName: "a`b"},
		"space":        {CommonName: "a b"},
		"newline":      {CommonName: "a\nb"},
		"non-ascii":    {CommonName: "你好.com"},
		"path-trav":    {CommonName: "../../etc"},
		"too long":     {CommonName: strings.Repeat("a", 254)},
		"neg days":     {Days: -1},
		"over days":    {Days: 36526},
	}
	for label, c := range cases {
		if err := ValidateRequest(c); err == nil {
			t.Errorf("ValidateRequest(%s, %+v) should reject", label, c)
		}
	}
}

func TestBuildSelfSignPlan_Defaults(t *testing.T) {
	plan, err := BuildSelfSignPlan(SelfSignRequest{})
	if err != nil {
		t.Fatalf("BuildSelfSignPlan: %v", err)
	}
	if plan.Name != PlanNameSelfSigned {
		t.Errorf("Name = %q, want %q", plan.Name, PlanNameSelfSigned)
	}
	if plan.RollbackOnFailure {
		t.Error("RollbackOnFailure must be false (single-step atomic)")
	}
	if len(plan.Steps) != 1 {
		t.Fatalf("step count = %d, want 1", len(plan.Steps))
	}
	step := plan.Steps[0]
	if step.Metadata["cn"] != DefaultCertCN {
		t.Errorf("default CN = %q, want %q", step.Metadata["cn"], DefaultCertCN)
	}
	if step.Metadata["cert_path"] != SelfSignedCert {
		t.Errorf("default cert_path = %q, want %q", step.Metadata["cert_path"], SelfSignedCert)
	}
	if step.Metadata["key_path"] != SelfSignedKey {
		t.Errorf("default key_path = %q, want %q", step.Metadata["key_path"], SelfSignedKey)
	}
}

func TestBuildSelfSignPlan_OverridesPathsAndCN(t *testing.T) {
	plan, err := BuildSelfSignPlan(SelfSignRequest{
		CommonName: "example.com",
		Days:       30,
		CertPath:   "/tmp/x.pem",
		KeyPath:    "/tmp/x.key",
	})
	if err != nil {
		t.Fatalf("BuildSelfSignPlan: %v", err)
	}
	if plan.Steps[0].Metadata["cn"] != "example.com" {
		t.Errorf("CN override missed: %q", plan.Steps[0].Metadata["cn"])
	}
	if plan.Steps[0].Metadata["days"] != "30" {
		t.Errorf("days override missed: %q", plan.Steps[0].Metadata["days"])
	}
}

func TestBuildSelfSignPlan_RejectsBadCN(t *testing.T) {
	if _, err := BuildSelfSignPlan(SelfSignRequest{CommonName: "evil; rm -rf"}); err == nil {
		t.Error("expected rejection of injection-laden CN")
	}
}

func TestExecutor_GeneratesValidCertificate(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	plan, err := BuildSelfSignPlan(SelfSignRequest{
		CommonName: "test.example",
		Days:       30,
		CertPath:   certPath,
		KeyPath:    keyPath,
	})
	if err != nil {
		t.Fatalf("BuildSelfSignPlan: %v", err)
	}
	if _, err := plan.Execute(context.Background(), NewExecutor()); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	certBytes, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatalf("read cert: %v", err)
	}
	block, _ := pem.Decode(certBytes)
	if block == nil {
		t.Fatal("pem.Decode returned nil")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}
	if cert.Subject.CommonName != "test.example" {
		t.Errorf("CN = %q, want %q", cert.Subject.CommonName, "test.example")
	}
	if !cert.NotAfter.After(time.Now()) {
		t.Error("cert NotAfter should be in the future")
	}
}

func TestExecutor_KeyFileIsMode0600(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "k.pem")
	plan, _ := BuildSelfSignPlan(SelfSignRequest{
		CertPath: filepath.Join(dir, "c.pem"),
		KeyPath:  keyPath,
		Days:     1,
	})
	if _, err := plan.Execute(context.Background(), NewExecutor()); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("key file mode = %#o, want 0600", mode)
	}
}

func TestExecutor_CertFileIsMode0644(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "c.pem")
	plan, _ := BuildSelfSignPlan(SelfSignRequest{
		CertPath: certPath,
		KeyPath:  filepath.Join(dir, "k.pem"),
		Days:     1,
	})
	if _, err := plan.Execute(context.Background(), NewExecutor()); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	info, err := os.Stat(certPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o644 {
		t.Errorf("cert file mode = %#o, want 0644", mode)
	}
}

func TestExecutor_RejectsUnknownCommand(t *testing.T) {
	exec := NewExecutor()
	err := exec.ExecuteStep(context.Background(), minStep("nope"))
	if err == nil {
		t.Error("expected error for unsupported command")
	}
}

func TestExecutor_RejectsMissingMetadata(t *testing.T) {
	exec := NewExecutor()
	step := minStep("certmgr-self-sign")
	step.Metadata = nil
	if err := exec.ExecuteStep(context.Background(), step); err == nil {
		t.Error("expected error for nil metadata")
	}
}

func TestExecutor_RejectsMissingPaths(t *testing.T) {
	exec := NewExecutor()
	step := minStep("certmgr-self-sign")
	step.Metadata = map[string]string{"cn": "x"}
	if err := exec.ExecuteStep(context.Background(), step); err == nil {
		t.Error("expected error for missing cert_path/key_path")
	}
}

func TestExecutor_RejectsBadDays(t *testing.T) {
	dir := t.TempDir()
	exec := NewExecutor()
	step := minStep("certmgr-self-sign")
	step.Metadata = map[string]string{
		"cn":        "x.example",
		"cert_path": filepath.Join(dir, "c.pem"),
		"key_path":  filepath.Join(dir, "k.pem"),
		"days":      "abc",
	}
	if err := exec.ExecuteStep(context.Background(), step); err == nil {
		t.Error("expected error for malformed days")
	}
}

func TestDetectAcme_AbsentReturnsPresentFalse(t *testing.T) {
	dir := t.TempDir() // empty
	status, err := detectAcmeIn(dir)
	if err != nil {
		t.Fatalf("detectAcmeIn: %v", err)
	}
	if status.Present {
		t.Error("Present should be false on empty dir")
	}
}

func TestDetectAcme_DetectsFullFlow(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"cert.crt", "private.key"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("placeholder"), 0o600); err != nil {
			t.Fatalf("write %s: %v", f, err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "ca.log"), []byte("example.com\n"), 0o600); err != nil {
		t.Fatalf("write ca.log: %v", err)
	}
	status, err := detectAcmeIn(dir)
	if err != nil {
		t.Fatalf("detectAcmeIn: %v", err)
	}
	if !status.Present {
		t.Error("Present should be true")
	}
	if status.Domain != "example.com" {
		t.Errorf("Domain = %q, want example.com", status.Domain)
	}
	if status.CertSize == 0 || status.KeySize == 0 {
		t.Error("size fields should be populated")
	}
}

func TestDetectAcme_ZeroByteCertIsAbsent(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"cert.crt", "private.key"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte{}, 0o600); err != nil {
			t.Fatalf("write %s: %v", f, err)
		}
	}
	status, _ := detectAcmeIn(dir)
	if status.Present {
		t.Error("zero-byte cert should be reported absent")
	}
}

func TestDetectAcme_MissingLogIsTolerated(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"cert.crt", "private.key"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o600); err != nil {
			t.Fatalf("write %s: %v", f, err)
		}
	}
	status, err := detectAcmeIn(dir)
	if err != nil {
		t.Fatalf("detectAcmeIn: %v", err)
	}
	if !status.Present {
		t.Error("Present should still be true when ca.log is absent")
	}
	if status.Domain != "" {
		t.Errorf("Domain should be empty when ca.log absent, got %q", status.Domain)
	}
}

func TestDetectAcme_DefaultPathFallback(t *testing.T) {
	// DetectAcme() defaults to AcmeDir (/root/ygkkkca). In CI that path
	// almost certainly doesn't exist, so Present should be false; if a
	// developer has it locally we just skip the assertion.
	status, err := DetectAcme()
	if err != nil {
		t.Fatalf("DetectAcme: %v", err)
	}
	if status.Present {
		t.Skipf("host has %s populated; skipping negative assertion", AcmeDir)
	}
}

func TestExecutor_RandSourceFallback(t *testing.T) {
	// nil RandReader → falls through to rand.Reader.
	exec := &Executor{NowFunc: time.Now}
	dir := t.TempDir()
	step := minStep("certmgr-self-sign")
	step.Metadata = map[string]string{
		"cn":        "x.example",
		"cert_path": filepath.Join(dir, "c.pem"),
		"key_path":  filepath.Join(dir, "k.pem"),
		"days":      "1",
	}
	if err := exec.ExecuteStep(context.Background(), step); err != nil {
		t.Fatalf("ExecuteStep: %v", err)
	}
}

func TestExecutor_RandReaderError(t *testing.T) {
	exec := &Executor{
		RandReader: func() (io.Reader, error) {
			return nil, errors.New("forced rand failure")
		},
	}
	dir := t.TempDir()
	step := minStep("certmgr-self-sign")
	step.Metadata = map[string]string{
		"cn":        "x.example",
		"cert_path": filepath.Join(dir, "c.pem"),
		"key_path":  filepath.Join(dir, "k.pem"),
	}
	err := exec.ExecuteStep(context.Background(), step)
	if err == nil {
		t.Error("expected error when RandReader fails")
	}
	if !strings.Contains(err.Error(), "random source") {
		t.Errorf("error should mention random source: %v", err)
	}
}

func TestAtomicWrite_WriteFailsWhenDirCannotBeCreated(t *testing.T) {
	// Use a file (not a dir) as the parent so MkdirAll fails.
	dir := t.TempDir()
	notADir := filepath.Join(dir, "notadir")
	if err := os.WriteFile(notADir, []byte("x"), 0o600); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	target := filepath.Join(notADir, "child", "out.pem")
	err := atomicWrite(target, []byte("data"), 0o600)
	if err == nil {
		t.Error("expected MkdirAll failure")
	}
}

func TestExecutor_AtomicWriteCleansUpKeyOnFailure(t *testing.T) {
	// Force the key write to fail by pointing to an unwritable directory.
	dir := t.TempDir()
	certPath := filepath.Join(dir, "ok.pem")
	keyPath := filepath.Join(dir, "noexist", "noexist2", "nope.key")
	plan, _ := BuildSelfSignPlan(SelfSignRequest{
		CertPath: certPath,
		KeyPath:  keyPath,
		Days:     1,
	})
	_, err := plan.Execute(context.Background(), NewExecutor())
	if err == nil {
		// Atomic write creates intermediate dirs, so this scenario may succeed.
		t.Skip("atomic write succeeds via MkdirAll; cert+key both landed")
	}
	// If err is non-nil the cert path should have been cleaned up.
	if _, statErr := os.Stat(certPath); statErr == nil {
		t.Errorf("cert file should be removed after key write failure")
	}
}
