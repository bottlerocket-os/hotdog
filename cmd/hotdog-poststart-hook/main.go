package main

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/bottlerocket/hotdog"
	"github.com/bottlerocket/hotdog/hook"

	selinux "github.com/opencontainers/selinux/go-selinux"
)

func main() {
	if err := _main(); err != nil {
		panic(err)
	}
}

func _main() error {
	state, err := hook.State()
	if err != nil {
		return err
	}
	spec, err := hook.Config(state)
	if err != nil {
		return err
	}
	if spec.Process.SelinuxLabel != "" {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		if err := selinux.SetExecLabel(spec.Process.SelinuxLabel); err != nil {
			return err
		}
	}
	capJSON, err := json.Marshal(spec.Process.Capabilities)
	if err != nil {
		return err
	}
	hotpatch := exec.Command("nsenter",
		"-t", strconv.Itoa(state.Pid),
		"-m", "-n", "-i", "-u", "-p",
		filepath.Join(hotdog.ContainerDir, hotdog.HotpatchBinary))
	hotpatch.Env = []string{hotdog.EnvCapability + "=" + string(capJSON)}
	return hotpatch.Start()
}
