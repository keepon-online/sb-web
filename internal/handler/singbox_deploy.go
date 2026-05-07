package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"miaomiaowu/internal/logger"
	"miaomiaowu/internal/singbox"
	"miaomiaowu/internal/storage"
)

// SingboxDeployRequest describes a sb.sh-like server deployment request.
type SingboxDeployRequest struct {
	ExternalHost       string `json:"external_host"`
	Hostname           string `json:"hostname"`
	UUID               string `json:"uuid,omitempty"`
	Password           string `json:"password,omitempty"`
	RealitySNI         string `json:"reality_sni,omitempty"`
	RealityPrivateKey  string `json:"reality_private_key,omitempty"`
	RealityPublicKey   string `json:"reality_public_key,omitempty"`
	RealityShortID     string `json:"reality_short_id,omitempty"`
	WebSocketPath      string `json:"websocket_path,omitempty"`
	CertificatePath    string `json:"certificate_path,omitempty"`
	PrivateKeyPath     string `json:"private_key_path,omitempty"`
	VlessRealityPort   int    `json:"vless_reality_port,omitempty"`
	VmessWebSocketPort int    `json:"vmess_websocket_port,omitempty"`
	Hysteria2Port      int    `json:"hysteria2_port,omitempty"`
	TUICPort           int    `json:"tuic_port,omitempty"`
	AnyTLSPort         int    `json:"anytls_port,omitempty"`
	ConfigName         string `json:"config_name,omitempty"`
}

// SingboxDeployResponse returns the saved config, generated secrets and client links.
type SingboxDeployResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	*singbox.ServerDeployResult
}

// NewSingboxDeployHandler creates a one-click server deployment preparation handler.
func NewSingboxDeployHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		var req SingboxDeployRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
			return
		}
		if strings.TrimSpace(req.ExternalHost) == "" {
			writeError(w, http.StatusBadRequest, errors.New("external_host is required"))
			return
		}

		username := getUsernameFromContext(r.Context())
		logOperation(repo, username, "singbox_deploy", fmt.Sprintf("准备 Sing-box 节点部署: %s", req.ExternalHost))

		result, err := singbox.NewServerDeployer().Prepare(singbox.ServerDeployOptions{
			ExternalHost:       req.ExternalHost,
			Hostname:           req.Hostname,
			UUID:               req.UUID,
			Password:           req.Password,
			RealitySNI:         req.RealitySNI,
			RealityPrivateKey:  req.RealityPrivateKey,
			RealityPublicKey:   req.RealityPublicKey,
			RealityShortID:     req.RealityShortID,
			WebSocketPath:      req.WebSocketPath,
			CertificatePath:    req.CertificatePath,
			PrivateKeyPath:     req.PrivateKeyPath,
			VlessRealityPort:   req.VlessRealityPort,
			VmessWebSocketPort: req.VmessWebSocketPort,
			Hysteria2Port:      req.Hysteria2Port,
			TUICPort:           req.TUICPort,
			AnyTLSPort:         req.AnyTLSPort,
			ConfigName:         req.ConfigName,
		})
		if err != nil {
			logger.Error("[Singbox API] 节点部署配置准备失败", "error", err)
			logOperationWithError(repo, username, "singbox_deploy", err.Error())
			writeError(w, http.StatusInternalServerError, fmt.Errorf("prepare deployment failed: %w", err))
			return
		}

		saveDeployRecord(repo, req, result)

		writeJSON(w, http.StatusOK, SingboxDeployResponse{
			Success:            true,
			Message:            "Sing-box 节点部署配置已生成",
			ServerDeployResult: result,
		})

		logger.Info("[Singbox API] 节点部署配置准备成功", "host", req.ExternalHost, "config", result.ConfigPath)
	})
}

func saveDeployRecord(repo *storage.TrafficRepository, req SingboxDeployRequest, result *singbox.ServerDeployResult) {
	if repo == nil || result == nil || result.Config == nil {
		return
	}

	configJSON, err := json.Marshal(result.Config)
	if err != nil {
		logger.Warn("[Singbox API] 部署配置序列化失败", "error", err)
		return
	}

	name := strings.TrimSuffix(filepath.Base(result.ConfigPath), filepath.Ext(result.ConfigPath))
	if name == "" {
		name = strings.TrimSuffix(req.ConfigName, filepath.Ext(req.ConfigName))
	}
	if name == "" {
		name = "sb"
	}

	singboxConfig := &storage.SingboxConfig{
		Name:       name,
		Protocol:   "server",
		Port:       firstInboundPort(result.Config),
		ConfigJSON: string(configJSON),
		Enabled:    true,
	}
	if err := repo.CreateSingboxConfig(singboxConfig); err != nil {
		logger.Warn("[Singbox API] 部署配置数据库记录创建失败", "error", err)
	}
}

func firstInboundPort(config *singbox.SingboxConfig) int {
	for _, inbound := range config.Inbounds {
		if inbound.ListenPort > 0 {
			return inbound.ListenPort
		}
	}
	return 0
}
