package cgroups

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// EnterCgroups writes the current process into the cgroups of
// the target process
func EnterCgroups(targetPID string) error {
	cgroups, err := parseCgroupFile(filepath.Join("/proc", targetPID, "/cgroup"))
	if err != nil {
		return nil
	}
	pid := os.Getpid()

	for sub, path := range cgroups {
		if err := os.WriteFile(filepath.Join("/sys/fs/cgroup/", sub, path, "tasks"), []byte(strconv.Itoa(pid)), 0); err != nil {
			return err
		}
	}
	return nil
}

// parseCgroupFile returns a map of strings, with the keys being the names of the cgroups
// controllers, and the values the path name of the control group in the hierarchy
// to which the process belongs.
func parseCgroupFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	cgroups := make(map[string]string)

	for s.Scan() {
		text := s.Text()
		// from cgroups(7):
		// /proc/[pid]/cgroup
		// ...
		// For each cgroup hierarchy ... there is one entry
		// containing three colon-separated fields of the form:
		//     hierarchy-ID:subsystem-list:cgroup-path
		parts := strings.SplitN(text, ":", 3)
		if len(parts) < 3 {
			return nil, fmt.Errorf("invalid cgroup entry: must contain at least two colons: %v", text)
		}
		subsystem := parts[1]
		path := parts[2]
		// The `cgroup` file contains lines with empty subsystems
		if subsystem == "" {
			continue
		}
		// There are subsystems with the form `name=<sub>`
		if strings.Contains(subsystem, "=") {
			subParts := strings.SplitN(subsystem, "=", 2)
			if len(subParts) < 2 {
				return nil, fmt.Errorf("invalid subsystem format, must have subsystem name: %s", parts[1])
			}
			subsystem = subParts[1]
		}
		cgroups[subsystem] = path
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return cgroups, nil
}
