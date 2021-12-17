package main

import (
	"bufio"
	"errors"
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
)

var jvmOpts = []string{"-Xint", "-XX:+UseSerialGC", "-Dlog4jFixerVerbose=false"}
var delays = []time.Duration{
	0,
	time.Second,
	5 * time.Second,
	10 * time.Second,
	30 * time.Second,
}

const processName = "java"

type jvmVersion string

const (
	java17 jvmVersion = "17"
	java11 jvmVersion = "11"
	java8  jvmVersion = "8"
)

var logger *log.Logger

func main() {
	if err := _main(); err != nil {
		panic(err)
	}
}

func _main() error {
	logFile, err := os.OpenFile(filepath.Join("/", "dev", "shm", "hotdog.log"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	logger = log.New(logFile, "", log.LstdFlags|log.LUTC)
	logger.Println("Starting hotpatch")

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

type jvm struct {
	pid     int
	path    string
	euid    uint32
	egid    uint32
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
		javaVersionCmd := exec.Command(exePath, "-version")
		versionOut, err := javaVersionCmd.CombinedOutput()
		if err != nil {
			logger.Printf("Failed to execute %q for %d", "java -version", pid)
			continue
		}
		euid, egid, err := findEUID(pid)
		if err != nil {
			logger.Printf("Failed to find EUID for %d: %v", pid, err)
		}
		jvms = append(jvms, &jvm{
			pid:     pid,
			path:    exePath,
			euid:    euid,
			egid:    egid,
			version: string(versionOut),
		})
	}
	return jvms
}

func findEUID(pid int) (uint32, uint32, error) {
	status, err := os.OpenFile(filepath.Join("/proc", strconv.Itoa(pid), "status"), os.O_RDONLY, 0)
	if err != nil {
		return 0, 0, err
	}
	scanner := bufio.NewScanner(status)
	var (
		uidLine string
		gidLine string
	)
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "Uid:") {
			uidLine = scanner.Text()
		}
		if strings.HasPrefix(scanner.Text(), "Gid:") {
			gidLine = scanner.Text()
		}
		if uidLine != "" && gidLine != "" {
			break
		}
	}
	if uidLine == "" || gidLine == "" {
		return 0, 0, errors.New("not found")
	}
	uidLine = strings.TrimPrefix(uidLine, "Uid:\t")
	uidStr := strings.SplitN(uidLine, "\t", 2)[0]
	uid, err := strconv.Atoi(uidStr)
	if err != nil {
		return 0, 0, err
	}
	gidLine = strings.TrimPrefix(gidLine, "Gid:\t")
	gidStr := strings.SplitN(gidLine, "\t", 2)[0]
	gid, err := strconv.Atoi(gidStr)
	if err != nil {
		return 0, 0, err
	}
	return uint32(uid), uint32(gid), nil
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
	patch := exec.Command(j.path, options...)
	patch.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{
			Uid: j.euid,
			Gid: j.egid,
		},
	}
	out, err := patch.CombinedOutput()
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
