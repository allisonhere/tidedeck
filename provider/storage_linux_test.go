//go:build linux

package provider

import "testing"

func TestUnescapeMount(t *testing.T) {
	if got := unescapeMount(`/mnt/my\040disk`); got != "/mnt/my disk" {
		t.Fatalf("unescapeMount = %q", got)
	}
	if got := unescapeMount(`/a\011b`); got != "/a\tb" {
		t.Fatalf("unescapeMount tab = %q", got)
	}
}

func TestPseudoFilesystem(t *testing.T) {
	for _, fstype := range []string{"proc", "sysfs", "tmpfs", "cgroup2", "devpts"} {
		if !pseudoFilesystem(fstype) {
			t.Fatalf("%q should be pseudo", fstype)
		}
	}
	for _, fstype := range []string{"ext4", "xfs", "btrfs", "zfs", "f2fs"} {
		if pseudoFilesystem(fstype) {
			t.Fatalf("%q should be a real filesystem", fstype)
		}
	}
}
