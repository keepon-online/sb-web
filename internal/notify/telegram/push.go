package telegram

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"miaomiaowu/internal/systemops"
)

// DefaultSBoxDir is where sb.sh stores subscription/share files on disk.
const DefaultSBoxDir = "/etc/s-box"

// Step metadata keys used by PushExecutor to recover business intent.
const (
	metaKeyFile      = "telegram_file"
	metaKeyHeader    = "telegram_header"
	metaKeySplitFrom = "telegram_split_from"
	metaKeySplitIdx  = "telegram_split_idx"
	metaKeySplitN    = "telegram_split_n"
)

// subscriptionFile describes a single push target.
type subscriptionFile struct {
	filename string // file under sboxDir, e.g. "vl_reality.txt"
	header   string // message prefix shown before the file payload
	splitN   int    // if >1, split file content into N HTML messages
}

// orderedFiles matches the legacy sb.sh order (vl_reality first, jhsub last).
var orderedFiles = []subscriptionFile{
	{filename: "vl_reality.txt", header: "🚀【 Vless-reality-vision 分享链接 】：支持v2rayng、nekobox"},
	{filename: "vm_ws.txt", header: "🚀【 Vmess-ws 分享链接 】：支持v2rayng、nekobox"},
	{filename: "vm_ws_argols.txt", header: "🚀【 Vmess-ws(tls)+Argo临时域名分享链接 】：支持v2rayng、nekobox"},
	{filename: "vm_ws_argogd.txt", header: "🚀【 Vmess-ws(tls)+Argo固定域名分享链接 】：支持v2rayng、nekobox"},
	{filename: "vm_ws_tls.txt", header: "🚀【 Vmess-ws-tls 分享链接 】：支持v2rayng、nekobox"},
	{filename: "hy2.txt", header: "🚀【 Hysteria-2 分享链接 】：支持v2rayng、nekobox"},
	{filename: "tuic5.txt", header: "🚀【 Tuic-v5 分享链接 】：支持nekobox"},
	{filename: "an.txt", header: "🚀【 Anytls 分享链接 】：仅最新内核可用"},
	{filename: "sing_box_gitlab.txt", header: "🚀【 Sing-box 订阅链接 】：支持SFA、SFW、SFI"},
	{filename: "sbox.json", header: "🚀【 Sing-box 配置文件(4段) 】：支持SFA、SFW、SFI", splitN: 4},
	{filename: "clash_meta_gitlab.txt", header: "🚀【 Mihomo 订阅链接 】：支持Mihomo相关客户端"},
	{filename: "clmi.yaml", header: "🚀【 Mihomo 配置文件(2段) 】：支持Mihomo相关客户端", splitN: 2},
	{filename: "jhsub.txt", header: "🚀【 聚合节点 】：支持nekobox"},
}

// BuildPushPlan scans sboxDir for known subscription files and assembles a
// systemops.OperationPlan with one step per file (or per split segment).
//
// `cfg` is validated but not embedded in step metadata — credentials never
// leave the in-memory PushExecutor.
//
// RollbackOnFailure is intentionally false: a partial push leaves earlier
// messages in the Telegram chat that cannot be unsent.
func BuildPushPlan(cfg Config, sboxDir string) (systemops.OperationPlan, error) {
	if err := cfg.Validate(); err != nil {
		return systemops.OperationPlan{}, err
	}
	if strings.TrimSpace(sboxDir) == "" {
		sboxDir = DefaultSBoxDir
	}

	var steps []systemops.OperationStep
	for _, sf := range orderedFiles {
		fullPath := filepath.Join(sboxDir, sf.filename)
		info, err := os.Stat(fullPath)
		if err != nil || info.IsDir() || info.Size() == 0 {
			continue
		}

		if sf.splitN > 1 {
			for i := 1; i <= sf.splitN; i++ {
				steps = append(steps, systemops.OperationStep{
					ID:     fmt.Sprintf("tg-push-%s-%d", sanitizeID(sf.filename), i),
					Title:  fmt.Sprintf("推送 %s (第 %d/%d 段)", sf.filename, i, sf.splitN),
					Kind:   systemops.StepKindSystem,
					Risk:   systemops.RiskLevelLow,
					Target: fullPath,
					Metadata: map[string]string{
						metaKeyFile:      fullPath,
						metaKeyHeader:    sf.header,
						metaKeySplitFrom: sf.filename,
						metaKeySplitIdx:  fmt.Sprintf("%d", i),
						metaKeySplitN:    fmt.Sprintf("%d", sf.splitN),
					},
				})
			}
			continue
		}

		steps = append(steps, systemops.OperationStep{
			ID:     fmt.Sprintf("tg-push-%s", sanitizeID(sf.filename)),
			Title:  fmt.Sprintf("推送 %s 到 Telegram", sf.filename),
			Kind:   systemops.StepKindSystem,
			Risk:   systemops.RiskLevelLow,
			Target: fullPath,
			Metadata: map[string]string{
				metaKeyFile:   fullPath,
				metaKeyHeader: sf.header,
			},
		})
	}

	if len(steps) == 0 {
		return systemops.OperationPlan{}, fmt.Errorf("telegram: no subscription files found under %s", sboxDir)
	}

	return systemops.OperationPlan{
		Name:              "telegram_push_subscriptions",
		Description:       "推送 Sing-box 订阅文件到 Telegram",
		RollbackOnFailure: false,
		Steps:             steps,
	}, nil
}

// PushExecutor implements systemops.StepExecutor by sending Telegram messages.
//
// It deliberately does NOT shell out to curl or execute the legacy sbtg.sh
// script; everything is plain Go net/http.
type PushExecutor struct {
	client *Client
}

// NewPushExecutor wires a Client into a systemops.StepExecutor.
func NewPushExecutor(client *Client) *PushExecutor {
	return &PushExecutor{client: client}
}

// ExecuteStep handles a single push step. Unknown step kinds delegate to a
// no-op so the executor is forward-compatible with future plan additions.
func (p *PushExecutor) ExecuteStep(ctx context.Context, step systemops.OperationStep) error {
	if p.client == nil {
		return fmt.Errorf("telegram: push executor has no client")
	}
	if step.Metadata == nil {
		return fmt.Errorf("telegram: step %q missing metadata", step.ID)
	}

	filePath := step.Metadata[metaKeyFile]
	if filePath == "" {
		return fmt.Errorf("telegram: step %q missing file path", step.ID)
	}

	raw, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("telegram: read %s: %w", filePath, err)
	}

	content := string(raw)
	if idxStr, ok := step.Metadata[metaKeySplitIdx]; ok {
		nStr := step.Metadata[metaKeySplitN]
		idx, e1 := parsePositive(idxStr)
		n, e2 := parsePositive(nStr)
		if e1 != nil || e2 != nil || idx < 1 || idx > n {
			return fmt.Errorf("telegram: invalid split metadata for step %q", step.ID)
		}
		content = sliceLines(content, idx, n)
	}

	header := step.Metadata[metaKeyHeader]
	var msg string
	if idxStr, ok := step.Metadata[metaKeySplitIdx]; ok && idxStr != "1" {
		// Subsequent split segments are payload-only, matching sb.sh behavior.
		msg = content
	} else if header != "" {
		msg = header + "\n\n" + content
	} else {
		msg = content
	}

	return p.client.SendMessage(ctx, msg)
}

// sliceLines returns segment `idx` (1-indexed) of `text` split into n roughly
// equal chunks by line count, matching the sb.sh head/tail arithmetic.
func sliceLines(text string, idx, n int) string {
	lines := strings.Split(text, "\n")
	total := len(lines)
	if n <= 1 || total == 0 {
		return text
	}
	chunk := total / n
	start := (idx - 1) * chunk
	end := idx * chunk
	if idx == n {
		end = total
	}
	if start < 0 {
		start = 0
	}
	if end > total {
		end = total
	}
	if start >= end {
		return ""
	}
	return strings.Join(lines[start:end], "\n")
}

func parsePositive(s string) (int, error) {
	var v int
	if _, err := fmt.Sscanf(s, "%d", &v); err != nil {
		return 0, err
	}
	if v <= 0 {
		return 0, fmt.Errorf("non-positive value: %d", v)
	}
	return v, nil
}

// sanitizeID strips characters that would make step IDs ambiguous.
func sanitizeID(name string) string {
	mapped := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '_', r == '-':
			return r
		}
		return '_'
	}, name)
	return strings.Trim(mapped, "_")
}
