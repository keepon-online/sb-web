package systemops

import "context"

type ServiceAction string

const (
	ServiceActionInstall ServiceAction = "install"
	ServiceActionStart   ServiceAction = "start"
	ServiceActionStop    ServiceAction = "stop"
	ServiceActionRestart ServiceAction = "restart"
	ServiceActionEnable  ServiceAction = "enable"
	ServiceActionDisable ServiceAction = "disable"
)

type ServiceDriver interface {
	ApplyServiceAction(ctx context.Context, serviceName string, action ServiceAction) error
	ServiceStatus(ctx context.Context, serviceName string) (ServiceState, error)
	ServiceLogs(ctx context.Context, serviceName string, lines int) (string, error)
}

type ServiceState struct {
	Running bool
	Enabled bool
	PID     int
}

type FirewallRule struct {
	Family      string
	Protocol    string
	SourcePort  string
	TargetPort  int
	Description string
}

type FirewallDriver interface {
	ApplyFirewallRule(ctx context.Context, rule FirewallRule) error
	RemoveFirewallRule(ctx context.Context, rule FirewallRule) error
}

type SystemSetting struct {
	Key         string
	Value       string
	Description string
}

type SystemDriver interface {
	ApplySystemSetting(ctx context.Context, setting SystemSetting) error
	ReloadSystemSettings(ctx context.Context) error
	WriteFile(ctx context.Context, path string, content []byte, mode uint32) error
}

type ScheduleSpec struct {
	Name        string
	Expression  string
	Command     string
	Description string
}

type SchedulerDriver interface {
	EnsureSchedule(ctx context.Context, spec ScheduleSpec) error
	RemoveSchedule(ctx context.Context, name string) error
}

type BinaryInstallSpec struct {
	Name        string
	Version     string
	Source      string
	Destination string
	Checksum    string
}

type BinaryDriver interface {
	InstallBinary(ctx context.Context, spec BinaryInstallSpec) error
	RemoveBinary(ctx context.Context, path string) error
	BinaryVersion(ctx context.Context, path string) (string, error)
}
