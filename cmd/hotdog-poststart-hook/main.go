package main

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/bottlerocket/hotdog"
	"github.com/bottlerocket/hotdog/cgroups"
	"github.com/bottlerocket/hotdog/hook"

	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/selinux/go-selinux"
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
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// Silently exit if:
	// - The process fails to constrain itself
	// - An error occurred while reading the container's capabilities
	// - An error occurred while the hotpatch was applied
	// We don't send these errors to the STDOUT because the runtime
	// only reads it when the hook errors out
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
	hotpatch.Env = []string{hotdog.EnvCapability + "=" + string(capJSON)}
	hotpatch.Start()
	return nil
}

// constrainProcess sets the SELinux label of the running process, and changes
// its cgroups to be the same as the target container.
func constrainProcess(spec specs.Spec, targetPID string) error {
	if err := cgroups.EnterCgroups(targetPID); err != nil {
		return err
	}
	if spec.Process.SelinuxLabel != "" {
		if err := selinux.SetExecLabel(spec.Process.SelinuxLabel); err != nil {
			return err
		}
	}
	return nil
}
