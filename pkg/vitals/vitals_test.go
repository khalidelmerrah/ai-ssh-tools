package vitals

import (
	"testing"
)

func TestParseMemoryBytes(t *testing.T) {
	sample := `               total        used        free      shared  buff/cache   available
Mem:      8123281408  2259972096  3987654656    12345678  1875654656  5543210987
Swap:     2147479552           0  2147479552`

	v := ParseMemoryBytes(sample)
	if v.Total != 8123281408 {
		t.Errorf("expected total 8123281408, got %d", v.Total)
	}
	if v.Used != 2259972096 {
		t.Errorf("expected used 2259972096, got %d", v.Used)
	}
	if v.Free != 3987654656 {
		t.Errorf("expected free 3987654656, got %d", v.Free)
	}
	if v.PercentUsed <= 0 {
		t.Errorf("expected percent used > 0, got %f", v.PercentUsed)
	}
}

func TestParseDisks(t *testing.T) {
	sample := `Filesystem     1B-blocks        Used   Available Use% Mounted on
/dev/root    80300892160 32120356864 48180535296  40% /
/dev/sda1      256000000      256000   255744000   1% /boot/efi`

	disks := ParseDisks(sample)
	if len(disks) != 2 {
		t.Fatalf("expected 2 disks, got %d", len(disks))
	}
	if disks[0].Mount != "/" {
		t.Errorf("expected mount /, got %s", disks[0].Mount)
	}
	if disks[0].TotalBytes != 80300892160 {
		t.Errorf("expected total 80300892160, got %d", disks[0].TotalBytes)
	}
}

func TestParseLoadAverages(t *testing.T) {
	sample := "0.08 0.12 0.15 1/450 12345"
	loads := ParseLoadAverages(sample)
	if len(loads) != 3 {
		t.Fatalf("expected 3 load averages, got %d", len(loads))
	}
	if loads[0] != 0.08 || loads[1] != 0.12 || loads[2] != 0.15 {
		t.Errorf("unexpected loads: %v", loads)
	}
}

func TestParseUptime(t *testing.T) {
	sample := "2977697.52 11845234.12"
	uptime := ParseUptime(sample)
	if uptime != 2977697 {
		t.Errorf("expected uptime 2977697, got %d", uptime)
	}
}

func TestParseOSName(t *testing.T) {
	sample := `NAME="Ubuntu"
VERSION="22.04.5 LTS (Jammy Jellyfish)"
ID=ubuntu
PRETTY_NAME="Ubuntu 22.04.5 LTS"`

	osName := ParseOSName(sample)
	if osName != "Ubuntu 22.04.5 LTS" {
		t.Errorf("expected 'Ubuntu 22.04.5 LTS', got %q", osName)
	}
}
