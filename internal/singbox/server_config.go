package singbox

import (
	"fmt"
)

// ServerConfigOptions describes the server-side multi-protocol sing-box
// configuration that sb.sh installs as /etc/s-box/sb.json.
type ServerConfigOptions struct {
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
}

// BuildServerConfig generates a server-side five-protocol configuration:
// VLESS Reality, VMess WebSocket, Hysteria2, TUIC, and AnyTLS.
func BuildServerConfig(opts ServerConfigOptions) (*SingboxConfig, error) {
	if err := validateServerConfigOptions(opts); err != nil {
		return nil, err
	}

	wsPath := opts.WebSocketPath
	if wsPath == "" {
		wsPath = "/vmessws"
	}
	if wsPath[0] != '/' {
		wsPath = "/" + wsPath
	}

	realitySNI := opts.RealitySNI
	if realitySNI == "" {
		realitySNI = "apple.com"
	}

	tls := func() *TLSOptions {
		return &TLSOptions{
			Enabled:         true,
			ServerName:      realitySNI,
			CertificatePath: opts.CertificatePath,
			KeyPath:         opts.PrivateKeyPath,
			ALPN:            []string{"h3"},
		}
	}

	cfg := &SingboxConfig{
		Log: LogConfig{
			Level:     "info",
			Timestamp: true,
		},
		DNS: DNSConfig{
			Servers: []DNSServer{
				{Tag: "cf", Type: "udp", Server: "1.1.1.1"},
				{Tag: "local", Type: "local"},
			},
		},
		Inbounds: []InboundConfig{
			{
				Type:       "vless",
				Tag:        "vless-sb",
				Listen:     "::",
				ListenPort: opts.VlessRealityPort,
				Users: []map[string]interface{}{{
					"uuid": opts.UUID,
					"flow": "xtls-rprx-vision",
				}},
				TLS: &TLSOptions{
					Enabled:    true,
					ServerName: realitySNI,
					Reality: &RealityOptions{
						Enabled:    true,
						PrivateKey: opts.RealityPrivateKey,
						ShortID:    opts.RealityShortID,
					},
				},
			},
			{
				Type:       "vmess",
				Tag:        "vmess-ws-sb",
				Listen:     "::",
				ListenPort: opts.VmessWebSocketPort,
				Users: []map[string]interface{}{{
					"uuid":    opts.UUID,
					"alterId": 0,
				}},
				Transport: &TransportOptions{
					Type: "ws",
					Path: wsPath,
				},
			},
			{
				Type:       "hysteria2",
				Tag:        "hysteria2-sb",
				Listen:     "::",
				ListenPort: opts.Hysteria2Port,
				Users: []map[string]interface{}{{
					"name":     "hy2",
					"password": opts.Password,
				}},
				Masquerade: "https://bing.com",
				TLS:        tls(),
			},
			{
				Type:              "tuic",
				Tag:               "tuic5-sb",
				Listen:            "::",
				ListenPort:        opts.TUICPort,
				CongestionControl: "bbr",
				Users: []map[string]interface{}{{
					"uuid":     opts.UUID,
					"password": opts.Password,
				}},
				TLS: tls(),
			},
			{
				Type:       "anytls",
				Tag:        "anytls-sb",
				Listen:     "::",
				ListenPort: opts.AnyTLSPort,
				Users: []map[string]interface{}{{
					"password": opts.Password,
				}},
				TLS: tls(),
			},
		},
		Outbounds: []OutboundConfig{
			{Type: "direct", Tag: "direct"},
			{Type: "block", Tag: "block"},
		},
		Route: RouteConfig{
			Final:                 "direct",
			DefaultDomainResolver: "cf",
		},
	}

	return cfg, nil
}

func validateServerConfigOptions(opts ServerConfigOptions) error {
	required := map[string]string{
		"external host":       opts.ExternalHost,
		"uuid":                opts.UUID,
		"password":            opts.Password,
		"reality private key": opts.RealityPrivateKey,
		"reality public key":  opts.RealityPublicKey,
		"reality short id":    opts.RealityShortID,
		"certificate path":    opts.CertificatePath,
		"private key path":    opts.PrivateKeyPath,
	}
	for name, value := range required {
		if value == "" {
			return fmt.Errorf("%s is required", name)
		}
	}

	ports := []int{
		opts.VlessRealityPort,
		opts.VmessWebSocketPort,
		opts.Hysteria2Port,
		opts.TUICPort,
		opts.AnyTLSPort,
	}
	seen := map[int]bool{}
	for _, port := range ports {
		if port < 1 || port > 65535 {
			return fmt.Errorf("invalid port: %d", port)
		}
		if seen[port] {
			return fmt.Errorf("duplicate port: %d", port)
		}
		seen[port] = true
	}

	return nil
}
