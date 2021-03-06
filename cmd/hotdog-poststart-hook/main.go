package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/bottlerocket/hotdog"
	"github.com/bottlerocket/hotdog/cgroups"
	"github.com/bottlerocket/hotdog/hook"
	"github.com/bottlerocket/hotdog/process"
	"github.com/bottlerocket/hotdog/seccomp"

	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/selinux/go-selinux"
	"golang.org/x/sys/unix"
)

func main() {
	if err := _main(); err != nil {
		panic(err)
	}
}

func _main() error {
	// Fail if an error occurs while the container's state or config are retrieved
	state, err := hook.State()
	if err != nil {
		return err
	}
	spec, err := hook.Config(state)
	if err != nil {
		return err
	}
	targetPID := strconv.Itoa(state.Pid)

	// Don't proceed if the prestart hook failed to copy the required
	// artifacts so that we don't execute arbitrary binaries that could
	// be inside the container's filesystem
	if !sentryExists(state.Bundle) {
		return nil
	}

	// Silently exit if:
	// - An error occurred while fetching the container's seccomp profile
	// - The process fails to constrain itself
	// - An error occurred while reading the container's capabilities
	// - An error occurred while the hotpatch was applied
	// We don't send these errors to the STDOUT because the runtime
	// only reads it when the hook errors out

	// Get the seccomp filters from the target container
	filters, err := seccomp.GetSeccompFilter(state.Pid)
	if err != nil {
		return nil
	}
	filtersJSON, err := json.Marshal(filters)
	if err != nil {
		return nil
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := constrainProcess(spec, targetPID); err != nil {
		return nil
	}
	capJSON, err := json.Marshal(spec.Process.Capabilities)
	if err != nil {
		return nil
	}
	hotpatch := exec.Command("nsenter",
		"-t", targetPID,
		"-m", "-n", "-i", "-u", "-p",
		filepath.Join(hotdog.ContainerDir, hotdog.HotpatchBinary))
	hotpatch.Env = []string{
		hotdog.EnvCapability + "=" + string(capJSON),
		hotdog.EnvSeccompFilter + "=" + string(filtersJSON),
	}
	hotpatch.Start()
	return nil
}

// constrainProcess sets the SELinux label of the running process, changes
// its cgroups to be the same as the target container, and sets the
// `NO_NEW_PRIVS` flags to prevent the current process to get more
// privileges.
func constrainProcess(spec specs.Spec, targetPID string) error {
	if err := cgroups.EnterCgroups(targetPID); err != nil {
		return err
	}
	if err := process.ConstrainFileDescriptors(); err != nil {
		return err
	}
	if spec.Process.SelinuxLabel != "" {
		if err := selinux.SetExecLabel(spec.Process.SelinuxLabel); err != nil {
			return err
		}
	}
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return err
	}
	return nil
}

// sentryExists returns true if the sentry file created in the prestart
// hook in the container's bundle exists, and it is a regular file
func sentryExists(bundle string) bool {
	stat, err := os.Stat(filepath.Join(bundle, hotdog.HotdogBundleDir, hotdog.PostStartHookSentry))
	// Treat any error as if the sentry file doesn't exist
	return err == nil && stat.Mode().IsRegular()
}
