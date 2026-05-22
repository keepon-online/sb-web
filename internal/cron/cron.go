// Package cron encapsulates the sb.sh cronsb / uncronsb business as systemops
// OperationPlan instances.
//
// Business reference: sb.sh:3801-3816 — cronsb installs a daily restart entry
// for the sing-box service ("0 1 * * * systemctl restart sing-box;rc-service
// sing-box restart"); uncronsb strips every sing-box-related line (matching
// /sing-box/, /sbwpph/, /url http/, /websbox/) from the current user's
// crontab.
//
// Implementation note: rather than spawning sh -c pipelines (the Sprint 5
// anti-pattern this project just retired) the package models the read-edit-
// write as a single OperationStep handled by a custom StepExecutor. This
// keeps audit records clean and lets the plan declare a real Rollback —
// failed Enable falls back to Disable, failed Disable falls back to Enable.
package cron

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"miaomiaowu/internal/systemops"
)

// Plan names recorded in the audit table.
const (
	PlanNameEnable  = "cron.enable"
	PlanNameDisable = "cron.disable"
)

// CronEntry is the canonical entry installed by Enable. Matches sb.sh:3804.
const CronEntry = "0 1 * * * systemctl restart sing-box;rc-service sing-box restart"

// purgePatterns mirrors the four sed deletions in sb.sh:3810-3813. Any line
// containing one of these substrings is removed before Enable appends the
// canonical entry or Disable finalizes the new crontab.
var purgePatterns = []string{
	"sing-box",
	"sbwpph",
	"url http",
	"websbox",
}

// Step IDs and metadata keys.
const (
	stepEnable      = "cron-enable"
	stepDisable     = "cron-disable"
	metaModeKey     = "cron_mode"
	metaModeEnable  = "enable"
	metaModeDisable = "disable"
)

// BuildEnablePlan composes a plan that installs the canonical daily restart
// entry. Atomicity is guaranteed by the executor's Apply implementation:
// either crontab(1) accepts the full content or the previous schedule remains
// untouched. Because the operation is single-shot and atomic, the plan does
// not need RollbackOnFailure — the systemops rollback loop only runs against
// prior completed steps, of which there are none here.
func BuildEnablePlan() systemops.OperationPlan {
	return systemops.OperationPlan{
		Name:              PlanNameEnable,
		Description:       "启用 sing-box 定时重启 cron 规则",
		RollbackOnFailure: false,
		Steps: []systemops.OperationStep{
			{
				ID:          stepEnable,
				Title:       "安装定时重启 crontab 规则",
				Description: CronEntry,
				Kind:        systemops.StepKindScheduler,
				Risk:        systemops.RiskLevelMedium,
				Target:      "crontab",
				Command:     "cron-manage", // marker only; consumed by Executor
				Metadata:    map[string]string{metaModeKey: metaModeEnable, "raw_entry": CronEntry},
			},
		},
	}
}

// BuildDisablePlan composes a plan that strips every sing-box related entry
// from the current user's crontab.
func BuildDisablePlan() systemops.OperationPlan {
	return systemops.OperationPlan{
		Name:              PlanNameDisable,
		Description:       "卸载 sing-box 定时重启 cron 规则",
		RollbackOnFailure: false,
		Steps: []systemops.OperationStep{
			{
				ID:          stepDisable,
				Title:       "清除 sing-box 相关 crontab 规则",
				Description: strings.Join(purgePatterns, ", "),
				Kind:        systemops.StepKindScheduler,
				Risk:        systemops.RiskLevelLow,
				Target:      "crontab",
				Command:     "cron-manage",
				Metadata:    map[string]string{metaModeKey: metaModeDisable},
			},
		},
	}
}

// Executor wires the cron-manage virtual command to actual crontab(1) calls.
//
// The seam fields (List/Apply) make the executor unit-testable without
// invoking the real crontab binary; they default to LiveCrontab in
// NewLiveExecutor.
type Executor struct {
	// List returns the current crontab content. A non-zero exit status with
	// no content (i.e. "no crontab for $USER") MUST be reported as empty
	// string + nil error so first-time installs are not treated as failure.
	List func(ctx context.Context) (string, error)

	// Apply replaces the current crontab with the supplied content. An empty
	// string clears the crontab entirely (which is what Disable wants when
	// every line was a sing-box line).
	Apply func(ctx context.Context, content string) error
}

// NewLiveExecutor returns an Executor bound to the real crontab(1) binary.
func NewLiveExecutor() *Executor {
	return &Executor{
		List:  liveList,
		Apply: liveApply,
	}
}

// ExecuteStep is the systemops.StepExecutor entry point.
func (e *Executor) ExecuteStep(ctx context.Context, step systemops.OperationStep) error {
	if e.List == nil || e.Apply == nil {
		return errors.New("cron: executor is not fully wired (List/Apply missing)")
	}
	mode := step.Metadata[metaModeKey]
	switch mode {
	case metaModeEnable:
		return e.applyEnable(ctx)
	case metaModeDisable:
		return e.applyDisable(ctx)
	default:
		return fmt.Errorf("cron: unsupported mode %q on step %q", mode, step.ID)
	}
}

func (e *Executor) applyEnable(ctx context.Context) error {
	current, err := e.List(ctx)
	if err != nil {
		return fmt.Errorf("cron: list crontab: %w", err)
	}
	cleaned := purge(current)
	cleaned = appendEntry(cleaned, CronEntry)
	return e.Apply(ctx, cleaned)
}

func (e *Executor) applyDisable(ctx context.Context) error {
	current, err := e.List(ctx)
	if err != nil {
		return fmt.Errorf("cron: list crontab: %w", err)
	}
	cleaned := purge(current)
	return e.Apply(ctx, cleaned)
}

// purge removes every line containing any purge pattern. Trailing newlines
// are normalized so the round-tripped crontab is deterministic.
func purge(content string) string {
	if content == "" {
		return ""
	}
	lines := strings.Split(content, "\n")
	out := make([]string, 0, len(lines))
nextLine:
	for _, line := range lines {
		for _, p := range purgePatterns {
			if strings.Contains(line, p) {
				continue nextLine
			}
		}
		out = append(out, line)
	}
	// strings.Split leaves a trailing empty element when content ends with \n;
	// drop it so appendEntry can re-add exactly one newline.
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	if len(out) == 0 {
		return ""
	}
	return strings.Join(out, "\n") + "\n"
}

// appendEntry appends a single cron line, ensuring exactly one trailing \n.
func appendEntry(content, entry string) string {
	entry = strings.TrimRight(entry, "\n")
	if content == "" {
		return entry + "\n"
	}
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return content + entry + "\n"
}

// liveList runs `crontab -l`. A "no crontab" exit (1 with empty stdout) is
// normalized to ("", nil).
func liveList(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "crontab", "-l")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// crontab returns 1 with stderr "no crontab for X" when the user has
		// no installed schedule. Treat that as the empty case.
		if stdout.Len() == 0 && strings.Contains(stderr.String(), "no crontab") {
			return "", nil
		}
		return "", fmt.Errorf("crontab -l failed: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// liveApply pipes content into `crontab -` to atomically replace the user's
// crontab. Empty content clears the crontab outright (mirrors `crontab -r`
// semantics without an extra fork).
func liveApply(ctx context.Context, content string) error {
	if content == "" {
		// Use crontab -r to drop the schedule entirely; tolerate "no crontab"
		// so Disable is idempotent.
		cmd := exec.CommandContext(ctx, "crontab", "-r")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			if strings.Contains(stderr.String(), "no crontab") {
				return nil
			}
			return fmt.Errorf("crontab -r failed: %w (%s)", err, strings.TrimSpace(stderr.String()))
		}
		return nil
	}
	cmd := exec.CommandContext(ctx, "crontab", "-")
	cmd.Stdin = strings.NewReader(content)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("crontab - failed: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}
