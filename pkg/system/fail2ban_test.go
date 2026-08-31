package system

import (
	"testing"
)

func TestParseJailList(t *testing.T) {
	sampleOutput := "Status\n|- Number of jail:      4\n`- Jail list:   nginx-bad-request, nginx-botsearch, nginx-http-auth, sshd\n"
	list := parseJailList(sampleOutput)
	if len(list) != 4 {
		t.Fatalf("expected 4 jails, got %d", len(list))
	}
	expected := []string{"nginx-bad-request", "nginx-botsearch", "nginx-http-auth", "sshd"}
	for i, name := range expected {
		if list[i] != name {
			t.Errorf("expected jail %d to be %s, got %s", i, name, list[i])
		}
	}
}

func TestParseJailDetail(t *testing.T) {
	sampleOutput := "Status for the jail: sshd\n|- Filter\n|  |- Currently failed: 4\n|  |- Total failed:     35\n|  `- Journal matches:  _SYSTEMD_UNIT=sshd.service + _COMM=sshd\n`- Actions\n   |- Currently banned: 2\n   |- Total banned:     5\n   `- Banned IP list:   118.196.84.13 192.0.2.1\n"
	info := parseJailDetail("sshd", sampleOutput)
	if info.Name != "sshd" {
		t.Errorf("expected name sshd, got %s", info.Name)
	}
	if info.CurrentlyFailed != 4 {
		t.Errorf("expected currently failed 4, got %d", info.CurrentlyFailed)
	}
	if info.TotalFailed != 35 {
		t.Errorf("expected total failed 35, got %d", info.TotalFailed)
	}
	if info.CurrentlyBanned != 2 {
		t.Errorf("expected currently banned 2, got %d", info.CurrentlyBanned)
	}
	if info.TotalBanned != 5 {
		t.Errorf("expected total banned 5, got %d", info.TotalBanned)
	}
	if len(info.BannedIPs) != 2 || info.BannedIPs[0] != "118.196.84.13" || info.BannedIPs[1] != "192.0.2.1" {
		t.Errorf("unexpected banned IPs: %v", info.BannedIPs)
	}
}
