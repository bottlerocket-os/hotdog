package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/bottlerocket/hotdog"
	"github.com/bottlerocket/hotdog/hook"
	"github.com/opencontainers/runtime-spec/specs-go"
	"golang.org/x/sys/unix"
)

func main() {
	if err := _main(); err != nil {
		panic(err)
	}
}

const reexecArg = "reexec"

func _main() error {
	if len(os.Args) >= 2 && os.Args[1] == reexecArg {
		return reexeced_main()
	}
	return hook_main()
}

func hook_main() error {
	state, err := hook.State()
	if err != nil {
		return err
	}
	hookpid := os.Getpid()
	reexecPath := filepath.Join("/proc", strconv.Itoa(hookpid), "exe")
	reexec := exec.Command("nsenter", "-t", strconv.Itoa(state.Pid), "-m", reexecPath, reexecArg, state.Bundle)
	out, err := reexec.CombinedOutput()
	if err != nil {
		fmt.Println(string(out))
		return err
	}
	return nil
}

func reexeced_main() error {
	if len(os.Args) != 3 {
		return errors.New("wrong arg len")
	}
	bundle := os.Args[2]
	spec, err := hook.Config(specs.State{Bundle: bundle})
	if err != nil {
		return err
	}
	if spec.Root == nil || len(spec.Root.Path) == 0 {
		return errors.New("undefined root path")
	}
	dest := filepath.Join(spec.Root.Path, hotdog.HotdogContainerDir)
	if stat, err := os.Stat(dest); err != nil {
		if _, ok := err.(*os.PathError); !ok {
			// cannot hotpatch
			return nil
		}
		if err := os.Mkdir(dest, 0755); err != nil {
			return err
		}
	} else if !stat.IsDir() {
		// cannot hotpatch
		return nil
	}
	err = unix.Mount(hotdog.HotdogDirectory, dest, "bind", unix.MS_BIND|unix.MS_NODEV|unix.MS_NOATIME|unix.MS_RELATIME, "")
	if err != nil {
		return err
	}
	// remount readonly
	return unix.Mount(hotdog.HotdogDirectory, dest, "bind", unix.MS_REMOUNT|unix.MS_BIND|unix.MS_RDONLY, "")
}
