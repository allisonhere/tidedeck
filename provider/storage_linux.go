//go:build linux

package provider

import (
	"bufio"
	"context"
	"os"
	"strings"
	"syscall"

	"github.com/allisonhere/tideui"
)

// Storage builds a Linux filesystem-usage source from /proc/mounts and statfs.
// Pseudo filesystems are skipped.
func Storage() func(context.Context) ([]tideui.StorageMount, error) {
	return func(context.Context) ([]tideui.StorageMount, error) {
		file, err := os.Open("/proc/mounts")
		if err != nil {
			return nil, err
		}
		defer file.Close()

		var mounts []tideui.StorageMount
		seen := map[string]bool{}
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) < 3 {
				continue
			}
			mountpoint := unescapeMount(fields[1])
			fstype := fields[2]
			if pseudoFilesystem(fstype) || seen[mountpoint] {
				continue
			}
			var stat syscall.Statfs_t
			if err := syscall.Statfs(mountpoint, &stat); err != nil {
				continue
			}
			total := stat.Blocks * uint64(stat.Bsize)
			free := stat.Bavail * uint64(stat.Bsize)
			if total == 0 {
				continue
			}
			used := total - free
			seen[mountpoint] = true
			mounts = append(mounts, tideui.StorageMount{
				Path:        mountpoint,
				UsedPercent: 100 * float64(used) / float64(total),
				Used:        humanBytes(float64(used)),
				Total:       humanBytes(float64(total)),
			})
		}
		return mounts, scanner.Err()
	}
}

func pseudoFilesystem(fstype string) bool {
	switch fstype {
	case "proc", "sysfs", "devtmpfs", "devpts", "tmpfs", "cgroup", "cgroup2",
		"securityfs", "debugfs", "tracefs", "pstore", "efivarfs", "bpf",
		"autofs", "mqueue", "hugetlbfs", "fusectl", "configfs", "binfmt_misc",
		"rpc_pipefs", "nfsd", "ramfs", "squashfs", "nsfs":
		return true
	}
	return false
}

// unescapeMount decodes the octal escapes /proc/mounts uses for spaces and
// other special characters.
func unescapeMount(value string) string {
	replacer := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	return replacer.Replace(value)
}
