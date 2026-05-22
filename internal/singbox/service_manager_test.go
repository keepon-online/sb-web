package singbox

import (
	"testing"

	"miaomiaowu/internal/systemops"
)

func TestSystemdServiceManagerServiceActionPlan(t *testing.T) {
	manager := &SystemdServiceManager{
		serviceName: "sing-box",
		binPath:     "/usr/local/bin/sing-box",
		configPath:  "/etc/sing-box/config.json",
	}

	tests := []struct {
		name       string
		action     systemops.ServiceAction
		wantID     string
		wantTitle  string
		wantArg    string
		wantDryRun bool
	}{
		{name: "start", action: systemops.ServiceActionStart, wantID: "start-service", wantTitle: "Start sing-box service", wantArg: "start"},
		{name: "stop", action: systemops.ServiceActionStop, wantID: "stop-service", wantTitle: "Stop sing-box service", wantArg: "stop"},
		{name: "restart", action: systemops.ServiceActionRestart, wantID: "restart-service", wantTitle: "Restart sing-box service", wantArg: "restart"},
		{name: "enable", action: systemops.ServiceActionEnable, wantID: "enable-service", wantTitle: "Enable sing-box service", wantArg: "enable"},
		{name: "disable", action: systemops.ServiceActionDisable, wantID: "disable-service", wantTitle: "Disable sing-box service", wantArg: "disable", wantDryRun: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan, err := manager.ServiceActionPlan(test.action, test.wantDryRun)
			if err != nil {
				t.Fatalf("ServiceActionPlan() returned error: %v", err)
			}
			if err := plan.Validate(); err != nil {
				t.Fatalf("Validate() returned error: %v", err)
			}
			if plan.Name != test.wantTitle {
				t.Fatalf("plan name = %q, want %q", plan.Name, test.wantTitle)
			}
			if plan.DryRun != test.wantDryRun {
				t.Fatalf("dry-run = %v, want %v", plan.DryRun, test.wantDryRun)
			}
			if len(plan.Steps) != 1 {
				t.Fatalf("steps = %d, want 1", len(plan.Steps))
			}

			step := plan.Steps[0]
			if step.ID != test.wantID {
				t.Fatalf("step id = %q, want %q", step.ID, test.wantID)
			}
			if step.Title != test.wantTitle {
				t.Fatalf("step title = %q, want %q", step.Title, test.wantTitle)
			}
			if step.Kind != systemops.StepKindService {
				t.Fatalf("step kind = %q, want %q", step.Kind, systemops.StepKindService)
			}
			if step.Risk != systemops.RiskLevelMedium {
				t.Fatalf("step risk = %q, want %q", step.Risk, systemops.RiskLevelMedium)
			}
			if step.Target != "sing-box" {
				t.Fatalf("step target = %q, want sing-box", step.Target)
			}
			if step.Command != "systemctl" {
				t.Fatalf("step command = %q, want systemctl", step.Command)
			}
			wantArgs := []string{test.wantArg, "sing-box"}
			if len(step.Args) != len(wantArgs) {
				t.Fatalf("step args = %#v, want %#v", step.Args, wantArgs)
			}
			for index, wantArg := range wantArgs {
				if step.Args[index] != wantArg {
					t.Fatalf("step args = %#v, want %#v", step.Args, wantArgs)
				}
			}
			if step.Metadata["service_action"] != string(test.action) {
				t.Fatalf("service action metadata = %q, want %q", step.Metadata["service_action"], test.action)
			}
		})
	}
}

func TestSystemdServiceManagerServiceActionPlanRejectsUnsupportedActions(t *testing.T) {
	manager := &SystemdServiceManager{serviceName: "sing-box"}

	tests := []struct {
		name   string
		action systemops.ServiceAction
	}{
		{name: "empty", action: systemops.ServiceAction("")},
		{name: "install", action: systemops.ServiceActionInstall},
		{name: "unknown", action: systemops.ServiceAction("reload")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := manager.ServiceActionPlan(test.action, true); err == nil {
				t.Fatal("expected unsupported action error")
			}
		})
	}
}

func TestSystemdServiceManagerInstallPlan(t *testing.T) {
	manager := &SystemdServiceManager{
		serviceName: "sing-box",
		binPath:     "/usr/local/bin/sing-box",
		configPath:  "/etc/sing-box/config.json",
	}

	plan := manager.InstallPlan(true)
	if err := plan.Validate(); err != nil {
		t.Fatalf("Validate() returned error: %v", err)
	}
	if !plan.DryRun {
		t.Fatal("install plan should be dry-run")
	}
	if plan.Name != "Install sing-box systemd service" {
		t.Fatalf("plan name = %q, want install plan name", plan.Name)
	}
	if len(plan.Steps) != 2 {
		t.Fatalf("steps = %d, want 2", len(plan.Steps))
	}

	writeStep := plan.Steps[0]
	if writeStep.ID != "write-service-file" {
		t.Fatalf("write step id = %q, want write-service-file", writeStep.ID)
	}
	if writeStep.Kind != systemops.StepKindFile {
		t.Fatalf("write step kind = %q, want %q", writeStep.Kind, systemops.StepKindFile)
	}
	if writeStep.Risk != systemops.RiskLevelHigh {
		t.Fatalf("write step risk = %q, want %q", writeStep.Risk, systemops.RiskLevelHigh)
	}
	if writeStep.Target != "/etc/systemd/system/sing-box.service" {
		t.Fatalf("write step target = %q, want service file path", writeStep.Target)
	}
	if writeStep.Metadata["mode"] != "0644" {
		t.Fatalf("write step mode = %q, want 0644", writeStep.Metadata["mode"])
	}
	if writeStep.Metadata["content"] == "" {
		t.Fatal("write step should include service content")
	}

	reloadStep := plan.Steps[1]
	if reloadStep.ID != "reload-systemd" {
		t.Fatalf("reload step id = %q, want reload-systemd", reloadStep.ID)
	}
	if reloadStep.Kind != systemops.StepKindSystem {
		t.Fatalf("reload step kind = %q, want %q", reloadStep.Kind, systemops.StepKindSystem)
	}
	if reloadStep.Command != "systemctl" {
		t.Fatalf("reload step command = %q, want systemctl", reloadStep.Command)
	}
	wantArgs := []string{"daemon-reload"}
	if len(reloadStep.Args) != len(wantArgs) || reloadStep.Args[0] != wantArgs[0] {
		t.Fatalf("reload step args = %#v, want %#v", reloadStep.Args, wantArgs)
	}
}
