package singbox

import (
	"reflect"
	"testing"
)

func TestBuildUDPHopCommandsForRange(t *testing.T) {
	cmds, err := BuildUDPHopCommands(PortHopOptions{
		ProtocolPort: 10003,
		Ports:        []string{"20000:20010"},
		IPV6:         true,
	})
	if err != nil {
		t.Fatalf("BuildUDPHopCommands returned error: %v", err)
	}

	want := []CommandSpec{
		{Name: "iptables", Args: []string{"-t", "nat", "-A", "PREROUTING", "-p", "udp", "--dport", "20000:20010", "-j", "DNAT", "--to-destination", ":10003"}},
		{Name: "ip6tables", Args: []string{"-t", "nat", "-A", "PREROUTING", "-p", "udp", "--dport", "20000:20010", "-j", "DNAT", "--to-destination", ":10003"}},
	}
	if !reflect.DeepEqual(cmds, want) {
		t.Fatalf("commands = %#v, want %#v", cmds, want)
	}
}

func TestBuildUDPHopCommandsForSinglePort(t *testing.T) {
	cmds, err := BuildUDPHopCommands(PortHopOptions{
		ProtocolPort: 10004,
		Ports:        []string{"24443"},
	})
	if err != nil {
		t.Fatalf("BuildUDPHopCommands returned error: %v", err)
	}

	want := []CommandSpec{
		{Name: "iptables", Args: []string{"-t", "nat", "-A", "PREROUTING", "-p", "udp", "--dport", "24443", "-j", "DNAT", "--to-destination", ":10004"}},
	}
	if !reflect.DeepEqual(cmds, want) {
		t.Fatalf("commands = %#v, want %#v", cmds, want)
	}
}

func TestBuildUDPHopCommandsRejectsTargetPortAsHop(t *testing.T) {
	_, err := BuildUDPHopCommands(PortHopOptions{
		ProtocolPort: 10004,
		Ports:        []string{"10004"},
	})
	if err == nil {
		t.Fatal("expected error for protocol port used as hop port")
	}
}
