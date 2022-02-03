package hotdog

import (
	"encoding/json"
	"fmt"
	"os"
	"syscall"
)

var (
	HostDir          = "/usr/share/hotdog"
	ContainerDir     = "/dev/shm/.hotdog"
	JDK8Class        = "Log4jHotPatch"
	PatchPath        = "Log4jHotPatch.jar"
	HotpatchBinary   = "hotdog-hotpatch"
	EnvCapability    = "HOTDOG_CAPABILITIES"
	EnvSeccompFilter = "HOTDOG_SECCOMP_FILTER"
)

// GetFiltersFromEnv reads and parses the seccomp filters passed as an
// environment variable of the running process
func GetFiltersFromEnv() ([][]syscall.SockFilter, error) {
	filtersJSON := os.Getenv(EnvSeccompFilter)
	// Only check if the environment variable was set, but don't fail if
	// the filters array is empty
	if len(filtersJSON) == 0 {
		return nil, fmt.Errorf("No filters were passed in %s", EnvSeccompFilter)
	}
	var filters [][]syscall.SockFilter
	if err := json.Unmarshal([]byte(filtersJSON), &filters); err != nil {
		return nil, fmt.Errorf("Failed to unmarshal filters: %v", err)
	}
	return filters, nil
}
