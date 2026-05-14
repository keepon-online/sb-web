package singbox

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"miaomiaowu/internal/storage"
)

const publishedNodeSourceType = "singbox"

type PublishRequest struct {
	ConfigID         int64
	Protocols        []string
	Tags             []string
	Enabled          bool
	ExternalHost     string
	RealityPublicKey string
}

type PublishResult struct {
	Created []storage.Node `json:"created"`
	Updated []storage.Node `json:"updated"`
}

type PublishedNode struct {
	ID              int64    `json:"id"`
	NodeName        string   `json:"node_name"`
	Protocol        string   `json:"protocol"`
	Tags            []string `json:"tags"`
	Enabled         bool     `json:"enabled"`
	OriginalServer  string   `json:"original_server"`
	SourceRefName   string   `json:"source_ref_name"`
	SourceUpdatedAt string   `json:"source_updated_at"`
}

type NodePublisher struct {
	repo *storage.TrafficRepository
}

func NewNodePublisher(repo *storage.TrafficRepository) *NodePublisher {
	return &NodePublisher{repo: repo}
}

func (p *NodePublisher) PublishConfig(ctx context.Context, req PublishRequest) (PublishResult, error) {
	if p == nil || p.repo == nil {
		return PublishResult{}, fmt.Errorf("node publisher repository not initialized")
	}
	config, err := p.repo.GetSingboxConfig(req.ConfigID)
	if err != nil {
		return PublishResult{}, fmt.Errorf("load singbox config: %w", err)
	}
	adminUsername, err := p.repo.GetAdminUsername(ctx)
	if err != nil {
		return PublishResult{}, fmt.Errorf("load admin username: %w", err)
	}

	nodes, err := buildPublishedNodes(config, req, adminUsername)
	if err != nil {
		return PublishResult{}, err
	}
	if len(nodes) == 0 {
		return PublishResult{}, fmt.Errorf("no publishable nodes found")
	}

	var result PublishResult
	for _, node := range nodes {
		stored, created, err := p.repo.UpsertNodeBySource(ctx, node)
		if err != nil {
			return PublishResult{}, fmt.Errorf("upsert published node %s: %w", node.Protocol, err)
		}
		if created {
			result.Created = append(result.Created, stored)
		} else {
			result.Updated = append(result.Updated, stored)
		}
	}
	return result, nil
}

func (p *NodePublisher) ListPublishedNodes(ctx context.Context, configID int64) ([]PublishedNode, error) {
	if p == nil || p.repo == nil {
		return nil, fmt.Errorf("node publisher repository not initialized")
	}
	nodes, err := p.repo.ListNodesBySource(ctx, publishedNodeSourceType, strconv.FormatInt(configID, 10))
	if err != nil {
		return nil, err
	}
	published := make([]PublishedNode, 0, len(nodes))
	for _, node := range nodes {
		published = append(published, PublishedNode{
			ID:              node.ID,
			NodeName:        node.NodeName,
			Protocol:        node.Protocol,
			Tags:            node.Tags,
			Enabled:         node.Enabled,
			OriginalServer:  node.OriginalServer,
			SourceRefName:   node.SourceRefName,
			SourceUpdatedAt: node.SourceUpdatedAt,
		})
	}
	return published, nil
}

func (p *NodePublisher) DeletePublishedNode(ctx context.Context, nodeID int64) error {
	if p == nil || p.repo == nil {
		return fmt.Errorf("node publisher repository not initialized")
	}
	return p.repo.DeleteNodeBySourceID(ctx, nodeID, publishedNodeSourceType)
}

func buildPublishedNodes(config storage.SingboxConfig, req PublishRequest, adminUsername string) ([]storage.Node, error) {
	var sbConfig SingboxConfig
	if err := json.Unmarshal([]byte(config.ConfigJSON), &sbConfig); err != nil {
		return nil, fmt.Errorf("parse singbox config JSON: %w", err)
	}

	opts, protocols, err := optionsFromServerConfig(sbConfig, req)
	if err != nil {
		return nil, err
	}
	links, err := GenerateShareLinks(opts)
	if err != nil {
		return nil, fmt.Errorf("generate share links: %w", err)
	}
	for protocol := range links {
		if !protocols[protocol] {
			delete(links, protocol)
		}
	}
	selected := normalizeProtocols(req.Protocols, links)
	if len(selected) == 0 {
		return nil, fmt.Errorf("no publishable protocols selected")
	}

	nodes := make([]storage.Node, 0, len(selected))
	for _, protocol := range selected {
		link := strings.TrimSpace(links[protocol])
		if link == "" {
			return nil, fmt.Errorf("protocol %s is not publishable", protocol)
		}
		nodeName := fmt.Sprintf("%s-%s", config.Name, protocol)
		clashConfig := clashConfigFromServerOptions(opts, protocol, nodeName)
		parsed := copyConfigMap(clashConfig)
		parsed["raw"] = link
		parsedJSON, err := json.Marshal(parsed)
		if err != nil {
			return nil, fmt.Errorf("marshal %s parsed config: %w", protocol, err)
		}
		clashJSON, err := json.Marshal(clashConfig)
		if err != nil {
			return nil, fmt.Errorf("marshal %s clash config: %w", protocol, err)
		}
		nodes = append(nodes, storage.Node{
			Username:       adminUsername,
			RawURL:         link,
			NodeName:       nodeName,
			Protocol:       protocol,
			ParsedConfig:   string(parsedJSON),
			ClashConfig:    string(clashJSON),
			Enabled:        req.Enabled,
			Tags:           mergePublishTags(config.Name, protocol, req.Tags),
			OriginalServer: opts.ExternalHost,
			SourceType:     publishedNodeSourceType,
			SourceRefID:    strconv.FormatInt(config.ID, 10),
			SourceRefName:  config.Name,
		})
	}
	return nodes, nil
}

func optionsFromServerConfig(config SingboxConfig, req PublishRequest) (ServerConfigOptions, map[string]bool, error) {
	var opts ServerConfigOptions
	protocols := map[string]bool{}
	for _, inbound := range config.Inbounds {
		protocol := normalizePublishProtocol(inbound.Type)
		if !isPublishableProtocol(protocol) {
			continue
		}
		protocols[protocol] = true
		switch protocol {
		case "vless":
			opts.VlessRealityPort = inbound.ListenPort
			opts.UUID = firstNonEmpty(opts.UUID, userString(inbound, "uuid"))
			if inbound.TLS != nil {
				opts.RealitySNI = firstNonEmpty(opts.RealitySNI, inbound.TLS.ServerName)
				if inbound.TLS.Reality != nil {
					opts.RealityPrivateKey = firstNonEmpty(opts.RealityPrivateKey, inbound.TLS.Reality.PrivateKey)
					opts.RealityShortID = firstNonEmpty(opts.RealityShortID, inbound.TLS.Reality.ShortID)
				}
			}
		case "vmess":
			opts.VmessWebSocketPort = inbound.ListenPort
			opts.UUID = firstNonEmpty(opts.UUID, userString(inbound, "uuid"))
			if inbound.Transport != nil {
				opts.WebSocketPath = firstNonEmpty(opts.WebSocketPath, inbound.Transport.Path)
			}
		case "hysteria2":
			opts.Hysteria2Port = inbound.ListenPort
			opts.Password = firstNonEmpty(opts.Password, userString(inbound, "password"), inbound.Password)
		case "tuic":
			opts.TUICPort = inbound.ListenPort
			opts.UUID = firstNonEmpty(opts.UUID, userString(inbound, "uuid"))
			opts.Password = firstNonEmpty(opts.Password, userString(inbound, "password"))
		case "anytls":
			opts.AnyTLSPort = inbound.ListenPort
			opts.Password = firstNonEmpty(opts.Password, userString(inbound, "password"), inbound.Password)
		}
	}
	if len(protocols) == 0 {
		return ServerConfigOptions{}, nil, fmt.Errorf("no publishable protocols found")
	}
	selectedProtocols := selectedProtocolSet(req.Protocols, protocols)
	if opts.VlessRealityPort == 0 {
		opts.VlessRealityPort = fallbackPort(protocols, "vless", 10001)
	}
	if opts.VmessWebSocketPort == 0 {
		opts.VmessWebSocketPort = fallbackPort(protocols, "vmess", 10002)
	}
	if opts.Hysteria2Port == 0 {
		opts.Hysteria2Port = fallbackPort(protocols, "hysteria2", 10003)
	}
	if opts.TUICPort == 0 {
		opts.TUICPort = fallbackPort(protocols, "tuic", 10004)
	}
	if opts.AnyTLSPort == 0 {
		opts.AnyTLSPort = fallbackPort(protocols, "anytls", 10005)
	}
	externalHost := strings.TrimSpace(req.ExternalHost)
	if externalHost == "" {
		return ServerConfigOptions{}, nil, fmt.Errorf("external host is required for publishing")
	}
	realityPublicKey := strings.TrimSpace(req.RealityPublicKey)
	if selectedProtocols["vless"] && realityPublicKey == "" {
		return ServerConfigOptions{}, nil, fmt.Errorf("reality public key is required for publishing vless")
	}
	if realityPublicKey == "" {
		// vless not selected; satisfy BuildServerConfig validation. The placeholder is
		// confined to ServerConfigOptions and never reaches downstream Clash/share-link output.
		realityPublicKey = "unused-public-key"
	}
	opts.ExternalHost = externalHost
	opts.Hostname = externalHost
	opts.UUID = firstNonEmpty(opts.UUID, "00000000-0000-4000-8000-000000000000")
	opts.Password = firstNonEmpty(opts.Password, "singbox")
	opts.RealitySNI = firstNonEmpty(opts.RealitySNI, "apple.com")
	opts.RealityPrivateKey = firstNonEmpty(opts.RealityPrivateKey, "private-key")
	opts.RealityPublicKey = realityPublicKey
	opts.RealityShortID = firstNonEmpty(opts.RealityShortID, "abcd1234")
	opts.WebSocketPath = firstNonEmpty(opts.WebSocketPath, "/vmessws")
	opts.CertificatePath = "/etc/s-box/cert.pem"
	opts.PrivateKeyPath = "/etc/s-box/private.key"
	return opts, protocols, nil
}

func clashConfigFromServerOptions(opts ServerConfigOptions, protocol, nodeName string) map[string]any {
	config := map[string]any{
		"name":   nodeName,
		"type":   protocol,
		"server": opts.ExternalHost,
		"udp":    true,
	}
	switch protocol {
	case "vless":
		config["port"] = opts.VlessRealityPort
		config["uuid"] = opts.UUID
		config["network"] = "tcp"
		config["tls"] = true
		config["flow"] = "xtls-rprx-vision"
		config["encryption"] = "none"
		config["servername"] = defaultRealitySNI(opts.RealitySNI)
		config["client-fingerprint"] = "chrome"
		config["skip-cert-verify"] = false
		config["reality-opts"] = map[string]any{
			"public-key": opts.RealityPublicKey,
			"short-id":   opts.RealityShortID,
		}
	case "vmess":
		config["port"] = opts.VmessWebSocketPort
		config["uuid"] = opts.UUID
		config["alterId"] = 0
		config["cipher"] = "auto"
		config["network"] = "ws"
		config["tls"] = false
		config["ws-opts"] = map[string]any{
			"path": defaultWebSocketPath(opts.WebSocketPath),
			"headers": map[string]any{
				"Host": opts.ExternalHost,
			},
		}
	case "hysteria2":
		config["port"] = opts.Hysteria2Port
		config["password"] = opts.Password
		config["sni"] = defaultRealitySNI(opts.RealitySNI)
		config["alpn"] = []string{"h3"}
		config["skip-cert-verify"] = false
	case "tuic":
		config["port"] = opts.TUICPort
		config["uuid"] = opts.UUID
		config["password"] = opts.Password
		config["sni"] = defaultRealitySNI(opts.RealitySNI)
		config["alpn"] = []string{"h3"}
		config["skip-cert-verify"] = false
		config["congestion-controller"] = "bbr"
		config["udp-relay-mode"] = "native"
	case "anytls":
		config["port"] = opts.AnyTLSPort
		config["password"] = opts.Password
		config["sni"] = defaultRealitySNI(opts.RealitySNI)
		config["skip-cert-verify"] = false
	}
	return config
}

func copyConfigMap(config map[string]any) map[string]any {
	copied := make(map[string]any, len(config))
	for key, value := range config {
		copied[key] = deepCopyConfigValue(value)
	}
	return copied
}

func deepCopyConfigValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		return copyConfigMap(v)
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = deepCopyConfigValue(item)
		}
		return out
	case []string:
		out := make([]string, len(v))
		copy(out, v)
		return out
	default:
		return value
	}
}

func defaultRealitySNI(sni string) string {
	return firstNonEmpty(sni, "apple.com")
}

func defaultWebSocketPath(path string) string {
	path = firstNonEmpty(path, "/vmessws")
	if !strings.HasPrefix(path, "/") {
		return "/" + path
	}
	return path
}

func normalizeProtocols(requested []string, links map[string]string) []string {
	order := []string{"vless", "vmess", "hysteria2", "tuic", "anytls"}
	wanted := make(map[string]bool)
	for _, protocol := range requested {
		protocol = normalizePublishProtocol(protocol)
		if protocol != "" {
			wanted[protocol] = true
		}
	}
	selected := make([]string, 0, len(order))
	for _, protocol := range order {
		if links[protocol] == "" {
			continue
		}
		if len(wanted) == 0 || wanted[protocol] {
			selected = append(selected, protocol)
		}
	}
	return selected
}

func selectedProtocolSet(requested []string, available map[string]bool) map[string]bool {
	selected := make(map[string]bool)
	wanted := make(map[string]bool)
	for _, protocol := range requested {
		protocol = normalizePublishProtocol(protocol)
		if protocol != "" {
			wanted[protocol] = true
		}
	}
	for protocol := range available {
		if len(wanted) == 0 || wanted[protocol] {
			selected[protocol] = true
		}
	}
	return selected
}

func normalizePublishProtocol(protocol string) string {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	if protocol == "hy2" {
		return "hysteria2"
	}
	return protocol
}

func isPublishableProtocol(protocol string) bool {
	switch protocol {
	case "vless", "vmess", "hysteria2", "tuic", "anytls":
		return true
	default:
		return false
	}
}

func mergePublishTags(configName, protocol string, userTags []string) []string {
	base := []string{"singbox", "singbox:" + configName, "protocol:" + protocol}
	seen := make(map[string]bool)
	tags := make([]string, 0, len(base)+len(userTags))
	for _, tag := range append(base, userTags...) {
		tag = strings.TrimSpace(tag)
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		tags = append(tags, tag)
	}
	return tags
}

func userString(inbound InboundConfig, key string) string {
	if len(inbound.Users) == 0 || inbound.Users[0] == nil {
		return ""
	}
	value, ok := inbound.Users[0][key]
	if !ok {
		return ""
	}
	return fmt.Sprint(value)
}

func fallbackPort(protocols map[string]bool, protocol string, port int) int {
	if protocols[protocol] {
		return port
	}
	return port
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
