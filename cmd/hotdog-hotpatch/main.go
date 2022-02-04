package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/bottlerocket/hotdog"
	"github.com/bottlerocket/hotdog/process"
	"github.com/bottlerocket/hotdog/seccomp"

	"github.com/opencontainers/runtime-spec/specs-go"
	"kernel.org/pub/linux/libs/security/libcap/cap"
)

var (
	// hotdogCaps are the minimum set of capabilities we must reserve for
	// hotdog-hotpatch to execute.  These capabilities should be dropped when
	// executing untrusted container executables.
	hotdogCaps = []cap.Value{
		cap.SYS_PTRACE,      // readlink /proc/<pid>/exe, bypassing UID check
		cap.DAC_OVERRIDE,    // ensure we can read files as root
		cap.DAC_READ_SEARCH, // ensure we can traverse into directories as root
		cap.SETGID,          // allow us to change GID
		cap.SETUID,          // allow us to change UID
		cap.SETPCAP,         // allow us to set process capabilities
	}
	logger *log.Logger
	delays = []time.Duration{
		0,
		time.Second,
		5 * time.Second,
		10 * time.Second,
		30 * time.Second,
	}
	jvmOpts = []string{"-Xint", "-XX:+UseSerialGC", "-Dlog4jFixerVerbose=false"}
)

const processName = "java"

type jvmVersion string

const (
	java17 jvmVersion = "17"
	java11 jvmVersion = "11"
	java8  jvmVersion = "8"
)

func main() {
	if err := _main(); err != nil {
		panic(err)
	}
}

const setFilterArgs = "set-filter"

func _main() error {
	if len(os.Args) > 2 && os.Args[1] == setFilterArgs {
		return set_filters_main(os.Args[2:])
	}
	return hotpatch_main()
}

// hotpatch_main is the main function executed by the poststart hook, it
// finds the processes to patch and applies the patch to them
func hotpatch_main() error {
	logFile, err := os.OpenFile(filepath.Join("/", "dev", "shm", "hotdog.log"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0655)
	if err != nil {
		return err
	}
	defer logFile.Close()
	logger = log.New(logFile, "", log.LstdFlags|log.LUTC)
	logger.Println("Starting hotpatch")

	if err := constrainHotdogCapabilities(); err != nil {
		logger.Printf("Failed to constrain hotdog's capabilities: %v", err)
		return err
	}
	for _, d := range delays {
		time.Sleep(d)
		logger.Printf("Starting hotpatch after %v delay", d)
		jvms := findJVMs()
		for _, j := range jvms {
			err := runHotpatch(j)
			if err != nil {
				logger.Printf("Patching %d failed: %v", j.pid, err)
			}
		}
	}
	return nil
}

// set_filters_main is the main function executed by hotdog-hotpatch's
// reexec'ed process. It sets the seccomp filters for the forked process
// before the final command is executed.
func set_filters_main(args []string) error {
	// Get the seccomp filters from the environment variable
	filters, err := hotdog.GetFiltersFromEnv()
	if err != nil {
		return fmt.Errorf("failed to get filters from stdin: %v", err)
	}
	// Set the seccomp filters before launching the final command
	if err := seccomp.SetSeccompFilters(filters); err != nil {
		return fmt.Errorf("failed to set filters: %v", err)
	}
	// Execute the command passed as arguments in the reexecution of
	// 'hotdog-hotpatch'. It should be safe to attempt to execute a binary
	// in here, since the caller process should already have reduced
	// capabilities, and a seccomp filter was already set at this point.
	command := exec.Command(args[0], args[1:]...)
	out, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to execute command '%v': %s", args, string(out))
	}
	fmt.Print(string(out))
	return nil
}

// constrainHotdogCapabilities reduces the permission set available to the
// hotdog-hotpatch process itself.  Before launching other processes, this
// permission set should be further reduced.  In order to successfully apply
// the hotpatch, this must be a superset of the capabilities granted to the
// hotpatch process.  We can assume that no process inside the target container
// has capabilities exceeding the bounding set defined in the bundle.
func constrainHotdogCapabilities() error {
	capJSON := os.Getenv(hotdog.EnvCapability)
	if len(capJSON) == 0 {
		logger.Println("cannot find container capabilities!")
		return errors.New(hotdog.EnvCapability + " empty")
	}
	var containerCapabilities specs.LinuxCapabilities
	err := json.Unmarshal([]byte(capJSON), &containerCapabilities)
	if err != nil {
		return fmt.Errorf("cannot unmarshal container capabilities: %w", err)
	}

	containerBSet := make([]cap.Value, 0)
	for _, name := range containerCapabilities.Bounding {
		v, err := cap.FromName(strings.ToLower(name))
		if err != nil {
			return fmt.Errorf("cannot parse %q: %w", name, err)
		}
		containerBSet = append(containerBSet, v)
	}

	set := cap.NewSet()
	if err := set.SetFlag(cap.Permitted, true, append(containerBSet, hotdogCaps...)...); err != nil {
		return fmt.Errorf("failed to set permitted caps: %w", err)
	}
	if err := set.Fill(cap.Effective, cap.Permitted); err != nil {
		return fmt.Errorf("failed to set effective caps: %w", err)
	}
	if err := set.ClearFlag(cap.Inheritable); err != nil {
		return fmt.Errorf("failed to set inheritable caps: %w", err)
	}

	logger.Printf("Reducing capabilities to: %q", set.String())
	if err := set.SetProc(); err != nil {
		return fmt.Errorf("failed to setpcap: %w", err)
	}
	if err := cap.ResetAmbient(); err != nil {
		return fmt.Errorf("failed to clear ambient caps: %w", err)
	}
	for i := 0; i < int(cap.MaxBits()); i++ {
		if ok, err := set.GetFlag(cap.Permitted, cap.Value(i)); err != nil || !ok {
			if err := cap.DropBound(cap.Value(i)); err != nil {
				return fmt.Errorf("failed to drop %s: %w", cap.Value(i).String(), err)
			}
		}
	}
	return nil
}

type jvm struct {
	pid     int
	path    string
	euid    int
	egid    int
	version string
}

func findJVMs() []*jvm {
	proc, err := ioutil.ReadDir("/proc")
	if err != nil {
		return nil
	}
	jvms := make([]*jvm, 0)
	for _, p := range proc {
		if !p.IsDir() {
			continue
		}
		cmdline, err := ioutil.ReadFile(filepath.Join("/proc", p.Name(), "cmdline"))
		if err != nil {
			continue
		}
		cmd := strings.SplitN(string(cmdline), string(rune(0)), 2)[0]
		if filepath.Base(cmd) != processName {
			continue
		}

		pid, err := strconv.Atoi(p.Name())
		if err != nil {
			continue
		}
		logger.Printf("Found %s: %d", processName, pid)
		exePath, err := os.Readlink(filepath.Join("/proc", p.Name(), "exe"))
		if err != nil {
			logger.Printf("Failed to readlink for %d", pid)
			continue
		}
		status, err := process.ParseProcessStatus(pid)
		if err != nil {
			logger.Printf("Failed to find EUID for %d: %v", pid, err)
			continue
		}
		versionOut, err := commandDroppedPrivs(exePath, []string{"-version"}, status.Uid, status.Gid, pid)
		if err != nil {
			logger.Printf("Failed to execute %q for %d: %v, %q", "java -version", pid, err, string(versionOut))
			continue
		}
		jvms = append(jvms, &jvm{
			pid:     pid,
			path:    exePath,
			euid:    status.Uid,
			egid:    status.Gid,
			version: string(versionOut),
		})
	}
	return jvms
}

// commandDroppedPrivs runs the specified program with the specified UID and
// GID but a reduced capability set.  For programs running with a non-zero UID,
// we drop all capabilities in every set including the bounding set.  For
// programs run as UID 0, match the capability sets of the target process.
// This allows the kernel's PTRACE_MODE_READ_FSCREDS check to pass when the
// hotpatch attempts to find the JVM's socket by reading /proc/<pid>/root/. The
// seccomp filter of the target process is set for all programs.
func commandDroppedPrivs(name string, arg []string, uid, gid, targetPID int) ([]byte, error) {
	reexecPath := filepath.Join("/proc", "self", "exe")
	// Create a launcher that reexecs hotdog-hotpatch, so that it can set the seccomp filters
	// before launching the target command
	cmd := cap.NewLauncher(reexecPath, append([]string{hotdog.HotpatchBinary, setFilterArgs, name}, arg...), nil)
	if uid != 0 {
		cmd.SetUID(uid)
		cmd.SetMode(cap.ModeNoPriv)
		logger.Printf("dropping all capabilities and switching UID to %d for %s", uid, name)
	}
	if gid != 0 {
		cmd.SetGroups(gid, nil)
	}
	pr, pw, err := os.Pipe()
	defer pw.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to allocate pipe for %q: %w", name, err)
	}
	buf := &bytes.Buffer{}
	go func() {
		io.Copy(buf, pr)
		pr.Close()
	}()
	cmd.Callback(func(attr *syscall.ProcAttr, i interface{}) error {
		// Capture stdout/stderr, since we can't rely on the exec package to
		// do it for us here.
		attr.Files[1] = pw.Fd()
		attr.Files[2] = pw.Fd()

		// cap.* functions called within the callback only affect the locked OS
		// thread used to launch the program
		if targetPID != 0 && uid == 0 {
			set, err := cap.GetPID(targetPID)
			if err != nil {
				return fmt.Errorf("failed to load caps for %d: %w", targetPID, err)
			}
			if err := set.ClearFlag(cap.Inheritable); err != nil {
				return fmt.Errorf("failed to clear inheritable set: %w", err)
			}
			iabSet, err := cap.IABGetPID(targetPID)
			if err != nil {
				return fmt.Errorf("failed to load IAB for %d: %w", targetPID, err)
			}
			if err := iabSet.SetProc(); err != nil {
				return fmt.Errorf("failed to set IAB: %w", err)
			}
			if err := cap.ResetAmbient(); err != nil {
				return fmt.Errorf("failed to reset ambient: %w", err)
			}
			if err := set.SetProc(); err != nil {
				return fmt.Errorf("failed to set caps: %w", err)
			}
			logger.Printf("setting caps to %q for %q", set.String(), name)
			logger.Printf("setting IAB to %q for %q", iabSet.String(), name)
		}
		return nil
	})
	pid, err := cmd.Launch(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to launch %q: %w", name, err)
	}
	proc := os.Process{Pid: pid}
	state, err := proc.Wait()
	if err != nil {
		return nil, fmt.Errorf("failed to wait on %q: %w", name, err)
	}
	if state.ExitCode() != 0 {
		err = &exec.ExitError{ProcessState: state}
	}

	versionOut := buf.Bytes()
	return versionOut, err
}

func runHotpatch(j *jvm) error {
	version, ok := findVersion(j)
	if !ok {
		logger.Printf("Unsupported Java version %q for %d", version, j.pid)
		return nil
	}
	patchPath := filepath.Join(hotdog.ContainerDir, hotdog.PatchPath)
	var options []string
	switch version {
	case java17, java11:
		options = append(jvmOpts, "-jar", patchPath, strconv.Itoa(j.pid))
	case java8:
		// Sometimes java is invoked as $JAVAHOME/jre/bin/java versus $JAVAHOME/bin/java, try to correct for this.
		bindir := filepath.Dir(j.path)
		basedir := filepath.Dir(bindir)
		dirname := filepath.Base(basedir)
		if dirname == "jre" {
			basedir = filepath.Dir(basedir)
		}
		options = append(jvmOpts, "-cp", filepath.Join(basedir, "lib", "tools.jar")+":"+patchPath, hotdog.JDK8Class, strconv.Itoa(j.pid))
	default:
		return nil
	}
	out, err := commandDroppedPrivs(j.path, options, j.euid, j.egid, j.pid)
	exitCode := 0
	if err != nil {
		if execErr, ok := err.(*exec.ExitError); ok {
			exitCode = execErr.ExitCode()
		}
	}
	logger.Printf("Patch exited %d: %q", exitCode, string(out))
	return err
}

func findVersion(j *jvm) (jvmVersion, bool) {
	split := strings.SplitN(j.version, " ", 4)
	kind := split[0]
	if kind != "openjdk" && kind != "java" {
		logger.Printf("Skipping unsupported JVM kind: %q for %d", kind, j.pid)
		return "", false
	}
	if len(split) < 3 {
		logger.Printf("Failed to locate version for %d", j.pid)
		return "", false
	}
	semver := split[2]
	if semver[0] == '"' {
		semver = semver[1:]
	}
	if semver[len(semver)-1] == '"' {
		semver = semver[:len(semver)-1]
	}
	parts := strings.SplitN(semver, ".", 3)
	switch {
	case parts[0] == "17":
		return java17, true
	case parts[0] == "11",
		parts[0] == "15":
		return java11, true
	case parts[0] == "1" && len(parts) > 1 && parts[1] == "8":
		return java8, true
	}
	return jvmVersion(parts[0]), false
}
