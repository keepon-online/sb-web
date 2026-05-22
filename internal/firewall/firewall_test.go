package firewall

import (
	"testing"

	"miaomiaowu/internal/systemops"
)

func TestBuildDisablePlan_Shape(t *testing.T) {
	plan := BuildDisablePlan()
	if plan.Name != PlanNameDisable {
		t.Errorf("Name = %q, want %q", plan.Name, PlanNameDisable)
	}
	if plan.RollbackOnFailure {
		t.Error("RollbackOnFailure must be false (terminal action)")
	}
	if plan.DryRun {
		t.Error("DryRun must default to false")
	}
	if got := len(plan.Steps); got != 15 {
		t.Errorf("step count = %d, want 15 (11 core + 4 apache)", got)
	}
	if err := plan.Validate(); err != nil {
		t.Errorf("plan.Validate(): %v", err)
	}
}

func TestBuildDisablePlan_EveryStepIsBestEffort(t *testing.T) {
	plan := BuildDisablePlan()
	for _, s := range plan.Steps {
		if !s.ContinueOnError {
			t.Errorf("step %s: ContinueOnError must be true (sb.sh tolerance)", s.ID)
		}
	}
}

func TestBuildDisablePlan_StepsTargetRealBinariesNotShellWrappers(t *testing.T) {
	// Sprint 3 refactor invariant: steps must invoke real binaries directly,
	// not "sh -c" wrappers. Audit logs/observability depend on this.
	plan := BuildDisablePlan()
	for _, s := range plan.Steps {
		if s.Command == "sh" {
			t.Errorf("step %s still wrapped in sh: %v", s.ID, s.Args)
		}
		if s.Command == "" {
			t.Errorf("step %s has empty Command", s.ID)
		}
	}
}

func TestBuildDisablePlan_ApacheStepsHaveRunIfGuard(t *testing.T) {
	plan := BuildDisablePlan()
	expected := map[string]bool{
		"apache-httpd-stop":      true,
		"apache-httpd-disable":   true,
		"apache-apache2-stop":    true,
		"apache-apache2-disable": true,
	}
	seen := 0
	for _, s := range plan.Steps {
		if expected[s.ID] {
			seen++
			if s.RunIfCommand != "which" {
				t.Errorf("step %s: RunIfCommand = %q, want which", s.ID, s.RunIfCommand)
			}
			if len(s.RunIfArgs) != 1 || s.RunIfArgs[0] != "apachectl" {
				t.Errorf("step %s: RunIfArgs = %v, want [apachectl]", s.ID, s.RunIfArgs)
			}
		}
	}
	if seen != len(expected) {
		t.Errorf("found %d apache steps, want %d", seen, len(expected))
	}
}

func TestBuildDisablePlan_NonApacheStepsHaveNoRunIfGuard(t *testing.T) {
	plan := BuildDisablePlan()
	for _, s := range plan.Steps {
		if len(s.ID) >= 7 && s.ID[:7] == "apache-" {
			continue
		}
		if s.RunIfCommand != "" {
			t.Errorf("non-apache step %s should not have RunIfCommand, got %q", s.ID, s.RunIfCommand)
		}
	}
}

func TestBuildDisablePlan_SpecificCommands(t *testing.T) {
	plan := BuildDisablePlan()
	want := map[string][]string{
		"firewalld-stop":                 {"systemctl", "stop", "firewalld.service"},
		"firewalld-disable":              {"systemctl", "disable", "firewalld.service"},
		"selinux-permissive":             {"setenforce", "0"},
		"ufw-disable":                    {"ufw", "disable"},
		"iptables-policy-input-accept":   {"iptables", "-P", "INPUT", "ACCEPT"},
		"iptables-policy-forward-accept": {"iptables", "-P", "FORWARD", "ACCEPT"},
		"iptables-policy-output-accept":  {"iptables", "-P", "OUTPUT", "ACCEPT"},
		"iptables-mangle-flush":          {"iptables", "-t", "mangle", "-F"},
		"iptables-flush":                 {"iptables", "-F"},
		"iptables-delete-chains":         {"iptables", "-X"},
		"netfilter-persistent-save":      {"netfilter-persistent", "save"},
		"apache-httpd-stop":              {"systemctl", "stop", "httpd.service"},
		"apache-httpd-disable":           {"systemctl", "disable", "httpd.service"},
		"apache-apache2-stop":            {"service", "apache2", "stop"},
		"apache-apache2-disable":         {"systemctl", "disable", "apache2"},
	}
	indexed := map[string]systemops.OperationStep{}
	for _, s := range plan.Steps {
		indexed[s.ID] = s
	}
	for id, expected := range want {
		s, ok := indexed[id]
		if !ok {
			t.Errorf("missing step %s", id)
			continue
		}
		full := append([]string{s.Command}, s.Args...)
		if len(full) != len(expected) {
			t.Errorf("step %s: cmd+args = %v, want %v", id, full, expected)
			continue
		}
		for i := range full {
			if full[i] != expected[i] {
				t.Errorf("step %s arg[%d] = %q, want %q", id, i, full[i], expected[i])
			}
		}
	}
}

func TestBuildDisablePlan_StepRiskLevelsAreValid(t *testing.T) {
	plan := BuildDisablePlan()
	for _, s := range plan.Steps {
		if !s.Risk.Valid() {
			t.Errorf("step %s: invalid risk %q", s.ID, s.Risk)
		}
		if !s.Kind.Valid() {
			t.Errorf("step %s: invalid kind %q", s.ID, s.Kind)
		}
	}
}
