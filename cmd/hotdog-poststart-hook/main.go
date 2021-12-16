package main

import (
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/bottlerocket/hotdog"
	"github.com/bottlerocket/hotdog/hook"
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

	hotpatch := exec.Command("nsenter",
		"-t", strconv.Itoa(state.Pid),
		"-m", "-n", "-i", "-u", "-p",
		filepath.Join(hotdog.ContainerDir, hotdog.HotpatchBinary))
	return hotpatch.Start()
}
