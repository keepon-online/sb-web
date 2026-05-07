package singbox

import (
	"fmt"
	"strconv"
	"strings"

	"miaomiaowu/internal/util"
)

// CommandSpec is a shell command represented without invoking a shell.
type CommandSpec struct {
	Name string
	Args []string
}

type PortHopOptions struct {
	ProtocolPort int
	Ports        []string
	IPV6         bool
}

// BuildUDPHopCommands builds DNAT commands used by sb.sh for Hysteria2/TUIC
// UDP port hopping and multi-port reuse.
func BuildUDPHopCommands(opts PortHopOptions) ([]CommandSpec, error) {
	if opts.ProtocolPort < 1 || opts.ProtocolPort > 65535 {
		return nil, fmt.Errorf("invalid protocol port: %d", opts.ProtocolPort)
	}
	if len(opts.Ports) == 0 {
		return nil, fmt.Errorf("at least one hop port is required")
	}

	commands := make([]CommandSpec, 0, len(opts.Ports)*2)
	for _, hopPort := range opts.Ports {
		if err := validateHopPortSpec(hopPort, opts.ProtocolPort); err != nil {
			return nil, err
		}

		args := []string{
			"-t", "nat", "-A", "PREROUTING",
			"-p", "udp",
			"--dport", hopPort,
			"-j", "DNAT",
			"--to-destination", ":" + strconv.Itoa(opts.ProtocolPort),
		}
		commands = append(commands, CommandSpec{Name: "iptables", Args: args})
		if opts.IPV6 {
			commands = append(commands, CommandSpec{Name: "ip6tables", Args: args})
		}
	}

	return commands, nil
}

func ApplyUDPHopCommands(opts PortHopOptions) error {
	commands, err := BuildUDPHopCommands(opts)
	if err != nil {
		return err
	}
	for _, command := range commands {
		if err := util.Execute(command.Name, command.Args...); err != nil {
			return fmt.Errorf("execute %s: %w", command.Name, err)
		}
	}
	return nil
}

func validateHopPortSpec(spec string, protocolPort int) error {
	if spec == "" {
		return fmt.Errorf("empty hop port")
	}
	parts := strings.Split(spec, ":")
	if len(parts) > 2 {
		return fmt.Errorf("invalid hop port spec: %s", spec)
	}

	start, err := strconv.Atoi(parts[0])
	if err != nil {
		return fmt.Errorf("invalid hop port: %s", spec)
	}
	end := start
	if len(parts) == 2 {
		end, err = strconv.Atoi(parts[1])
		if err != nil {
			return fmt.Errorf("invalid hop port range: %s", spec)
		}
	}
	if start < 1 || end > 65535 || start > end {
		return fmt.Errorf("invalid hop port range: %s", spec)
	}
	if protocolPort >= start && protocolPort <= end {
		return fmt.Errorf("hop port %s includes protocol port %d", spec, protocolPort)
	}
	return nil
}
