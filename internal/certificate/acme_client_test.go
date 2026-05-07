package certificate

import (
	"reflect"
	"testing"
)

func TestBuildACMEShellCommands(t *testing.T) {
	commands := buildACMEShellCommands(acmeShellOptions{
		ACMEPath: "/root/.acme.sh/acme.sh",
		Domain:   "example.com",
		Email:    "admin@example.com",
		CertPath: "/etc/s-box/certs/example.com.crt",
		KeyPath:  "/etc/s-box/certs/example.com.key",
		Staging:  true,
	})

	want := []acmeShellCommand{
		{
			Name: "/root/.acme.sh/acme.sh",
			Args: []string{"--issue", "--standalone", "-d", "example.com", "--accountemail", "admin@example.com", "--server", "letsencrypt_test", "--keylength", "ec-256", "--force"},
		},
		{
			Name: "/root/.acme.sh/acme.sh",
			Args: []string{"--install-cert", "-d", "example.com", "--ecc", "--fullchain-file", "/etc/s-box/certs/example.com.crt", "--key-file", "/etc/s-box/certs/example.com.key"},
		},
	}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("commands = %#v, want %#v", commands, want)
	}
}
