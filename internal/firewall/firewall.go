// Package firewall encapsulates firewall lifecycle operations (currently:
// disabling the host firewall) as systemops OperationPlan instances.
//
// The disable plan mirrors the close() function in sb.sh:177-197.
package firewall

import (
	"strings"

	"miaomiaowu/internal/systemops"
)

// PlanNameDisable is the canonical plan name persisted in audit records.
const PlanNameDisable = "firewall.disable"

// BuildDisablePlan returns an OperationPlan that disables firewalld, ufw,
// flushes iptables, persists the new rules, and (if apachectl is available)
// stops httpd/apache2.
//
// Behavior parity with sb.sh:177-197:
//   - Every shell line in sb.sh terminates with ">/dev/null 2>&1" so a missing
//     binary or already-stopped service does not abort the script. The Sprint 3
//     systemops core gained ContinueOnError, so each step now invokes the real
//     command directly and lets the executor record any error to StepResult
//     without aborting the plan. The audit log captures the actual binary/args
//     instead of an sh -c wrapper, improving observability.
//   - The apache block runs only when "command -v apachectl" succeeds. This is
//     expressed via RunIfCommand, which short-circuits the step (records
//     SkippedReason="run-if check failed") when the probe exits non-zero,
//     mirroring the "[[ -n $(apachectl -v 2>/dev/null) ]]" guard in sb.sh.
//
// RollbackOnFailure is false: disabling the firewall is a terminal action and
// the upstream script provides no inverse.
func BuildDisablePlan() systemops.OperationPlan {
	steps := []systemops.OperationStep{
		bestEffortStep("firewalld-stop", "停止 firewalld",
			systemops.StepKindService, systemops.RiskLevelHigh, "firewalld.service",
			"systemctl", "stop", "firewalld.service"),
		bestEffortStep("firewalld-disable", "禁用 firewalld 开机自启",
			systemops.StepKindService, systemops.RiskLevelHigh, "firewalld.service",
			"systemctl", "disable", "firewalld.service"),
		bestEffortStep("selinux-permissive", "setenforce 0 (SELinux permissive)",
			systemops.StepKindSystem, systemops.RiskLevelHigh, "selinux",
			"setenforce", "0"),
		bestEffortStep("ufw-disable", "禁用 ufw",
			systemops.StepKindFirewall, systemops.RiskLevelHigh, "ufw",
			"ufw", "disable"),
		bestEffortStep("iptables-policy-input-accept", "iptables INPUT 默认策略放行",
			systemops.StepKindFirewall, systemops.RiskLevelCritical, "iptables/filter/INPUT",
			"iptables", "-P", "INPUT", "ACCEPT"),
		bestEffortStep("iptables-policy-forward-accept", "iptables FORWARD 默认策略放行",
			systemops.StepKindFirewall, systemops.RiskLevelCritical, "iptables/filter/FORWARD",
			"iptables", "-P", "FORWARD", "ACCEPT"),
		bestEffortStep("iptables-policy-output-accept", "iptables OUTPUT 默认策略放行",
			systemops.StepKindFirewall, systemops.RiskLevelCritical, "iptables/filter/OUTPUT",
			"iptables", "-P", "OUTPUT", "ACCEPT"),
		bestEffortStep("iptables-mangle-flush", "清空 iptables mangle 表",
			systemops.StepKindFirewall, systemops.RiskLevelCritical, "iptables/mangle",
			"iptables", "-t", "mangle", "-F"),
		bestEffortStep("iptables-flush", "清空 iptables filter 规则",
			systemops.StepKindFirewall, systemops.RiskLevelCritical, "iptables/filter",
			"iptables", "-F"),
		bestEffortStep("iptables-delete-chains", "删除 iptables 自定义链",
			systemops.StepKindFirewall, systemops.RiskLevelCritical, "iptables/chains",
			"iptables", "-X"),
		bestEffortStep("netfilter-persistent-save", "持久化保存 netfilter 规则",
			systemops.StepKindFirewall, systemops.RiskLevelMedium, "netfilter-persistent",
			"netfilter-persistent", "save"),
		apacheGatedStep("apache-httpd-stop", "停止 httpd (apachectl 探测命中)", "httpd.service",
			"systemctl", "stop", "httpd.service"),
		apacheGatedStep("apache-httpd-disable", "禁用 httpd 开机自启 (apachectl 探测命中)", "httpd.service",
			"systemctl", "disable", "httpd.service"),
		apacheGatedStep("apache-apache2-stop", "停止 apache2 (apachectl 探测命中)", "apache2",
			"service", "apache2", "stop"),
		apacheGatedStep("apache-apache2-disable", "禁用 apache2 开机自启 (apachectl 探测命中)", "apache2.service",
			"systemctl", "disable", "apache2"),
	}

	return systemops.OperationPlan{
		Name:              PlanNameDisable,
		Description:       "关闭防火墙：停止 firewalld/ufw、放行 iptables 三链、清空规则、持久化保存；如检测到 apachectl 则一并停用 httpd/apache2。",
		DryRun:            false,
		RollbackOnFailure: false,
		Steps:             steps,
	}
}

// bestEffortStep produces a step whose failure is recorded but does not abort
// the plan (mirrors sb.sh's ">/dev/null 2>&1" tolerance).
func bestEffortStep(id, title string, kind systemops.StepKind, risk systemops.RiskLevel, target, command string, args ...string) systemops.OperationStep {
	return systemops.OperationStep{
		ID:              id,
		Title:           title,
		Description:     command + " " + strings.Join(args, " "),
		Kind:            kind,
		Risk:            risk,
		Target:          target,
		Command:         command,
		Args:            args,
		ContinueOnError: true,
	}
}

// apacheGatedStep produces a best-effort step guarded by "which apachectl".
// When apachectl is absent the step short-circuits with SkippedReason set by
// the systemops core. "which" is used over "command -v" because the latter is
// a shell builtin and not a stand-alone binary exec.Command can resolve.
func apacheGatedStep(id, title, target, command string, args ...string) systemops.OperationStep {
	step := bestEffortStep(id, title, systemops.StepKindService, systemops.RiskLevelMedium, target, command, args...)
	step.RunIfCommand = "which"
	step.RunIfArgs = []string{"apachectl"}
	return step
}
