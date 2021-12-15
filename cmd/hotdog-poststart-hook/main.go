package main

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/bottlerocket/hotdog"
	"github.com/opencontainers/runtime-spec/specs-go"
)

func main() {
	if err := _main(); err != nil {
		panic(err)
	}
}

func _main() error {
	stdinBytes, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		return err
	}
	var state specs.State
	err = json.Unmarshal(stdinBytes, &state)
	if err != nil {
		return err
	}

	hotpatch := exec.Command("nsenter",
		"-t", strconv.Itoa(state.Pid),
		"-m", "-n", "-i", "-u", "-p",
		filepath.Join(hotdog.HotdogContainerDir, "hotdog-hotpatch"))
	return hotpatch.Start()
}
