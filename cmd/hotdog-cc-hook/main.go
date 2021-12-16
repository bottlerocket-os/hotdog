package main

import (
	"errors"
	"fmt"
	"io"
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
	rootfs, err := hook.Root(bundle, spec)
	if err != nil {
		return err
	}

	hotdogBundleDir := filepath.Join(bundle, "hotdog")
	if err := os.Mkdir(hotdogBundleDir, 0755); err != nil {
		return err
	}
	if err := cp(filepath.Join(hotdog.HostDir, hotdog.JDK8Patch), filepath.Join(hotdogBundleDir, hotdog.JDK8Patch)); err != nil {
		return err
	}
	if err := cp(filepath.Join(hotdog.HostDir, hotdog.JDK11Patch), filepath.Join(hotdogBundleDir, hotdog.JDK11Patch)); err != nil {
		return err
	}
	if err := cp(filepath.Join(hotdog.HostDir, hotdog.JDK17Patch), filepath.Join(hotdogBundleDir, hotdog.JDK17Patch)); err != nil {
		return err
	}
	if err := cp(filepath.Join(hotdog.HostDir, hotdog.HotpatchBinary), filepath.Join(hotdogBundleDir, "hotdog-hotpatch")); err != nil {
		return err
	}

	mountTarget := filepath.Join(rootfs, hotdog.ContainerDir)
	if stat, err := os.Stat(mountTarget); err != nil {
		if _, ok := err.(*os.PathError); !ok {
			// cannot hotpatch
			return nil
		}
		if err := os.Mkdir(mountTarget, 0755); err != nil {
			return err
		}
	} else if !stat.IsDir() {
		// cannot hotpatch
		return nil
	}
	err = unix.Mount(hotdogBundleDir, mountTarget, "bind", unix.MS_BIND|unix.MS_NODEV|unix.MS_NOATIME|unix.MS_RELATIME, "")
	if err != nil {
		return err
	}
	// remount readonly
	return unix.Mount(hotdogBundleDir, mountTarget, "bind", unix.MS_REMOUNT|unix.MS_BIND|unix.MS_RDONLY, "")
}

func cp(in, out string) error {
	inReader, err := os.OpenFile(in, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	outWriter, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0755)
	if err != nil {
		return err
	}
	_, err = io.Copy(outWriter, inReader)
	return err
}
