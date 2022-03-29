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

	hotdogBundleDir := filepath.Join(bundle, hotdog.HotdogBundleDir)
	if err := os.Mkdir(hotdogBundleDir, 0755); err != nil {
		return err
	}
	// Copy the artifacts used in the poststart hook with the specified
	// permissions, so that child processes won't have the dumpable flag
	// set
	if err := cp(filepath.Join(hotdog.HostDir, hotdog.PatchPath), filepath.Join(hotdogBundleDir, hotdog.PatchPath), 0444); err != nil {
		return err
	}
	if err := cp(filepath.Join(hotdog.HostDir, hotdog.HotpatchBinary), filepath.Join(hotdogBundleDir, hotdog.HotpatchBinary), 0111); err != nil {
		return err
	}
	// Attempt to create mount target, checking that each part of 'hotdog.ContainerDir'
	// isn't a symlink
	if err := preparePath(rootfs, hotdog.ContainerDir); err != nil {
		return nil
	}

	mountTarget := filepath.Join(rootfs, hotdog.ContainerDir)
	err = unix.Mount(hotdogBundleDir, mountTarget, "bind", unix.MS_BIND|unix.MS_NODEV|unix.MS_NOATIME|unix.MS_RELATIME, "")
	if err != nil {
		// cannot hotpatch
		return nil
	}
	// remount readonly
	if err := unix.Mount(hotdogBundleDir, mountTarget, "bind", unix.MS_REMOUNT|unix.MS_BIND|unix.MS_RDONLY, ""); err != nil {
		return err
	}
	// Create sentry file used by the poststart hook to check if the binaries
	// were copied successfully
	sentry, err := os.Create(filepath.Join(hotdogBundleDir, hotdog.PostStartHookSentry))
	if err != nil {
		return err
	}
	return sentry.Close()
}

// preparePath creates the last directory in `path` under `root`, it returns
// an error if any of the parent parts in `path` are not directories, or if `path`
// exists.
func preparePath(root string, path string) error {
	fullPath := filepath.Join(root, path)
	// We use lstat(2) since `fullPath` could be a symlink. With this call
	// we read information about the link itself instead of the file the
	// link points to.
	if _, err := os.Lstat(fullPath); err == nil {
		// Don't use the path if it already exists
		return fmt.Errorf("Path exists: '%s'", fullPath)
	} else {
		// Fail if err is not `PathError`
		if _, ok := err.(*os.PathError); !ok {
			return err
		}
	}

	for parent := filepath.Dir(fullPath); parent != root; parent = filepath.Dir(parent) {
		// os.Lstat returns an error if the path doesn't exist
		if stat, err := os.Lstat(parent); err != nil {
			return err
		} else if !stat.IsDir() {
			// Fail if any parent is a symlink
			return fmt.Errorf("Path '%s' is not a directory", parent)
		}
	}
	return os.Mkdir(fullPath, 0755)
}

func cp(in, out string, mode os.FileMode) error {
	inReader, err := os.OpenFile(in, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer inReader.Close()
	outWriter, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	defer outWriter.Close()
	_, err = io.Copy(outWriter, inReader)
	return err
}
