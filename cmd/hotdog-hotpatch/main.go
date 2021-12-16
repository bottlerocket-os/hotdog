package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/bottlerocket/hotdog"
)

var jvmOpts = []string{"-Xint", "-XX:+UseSerialGC"}
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
			fmt.Println(j)
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
		jvms = append(jvms, &jvm{
			pid:     pid,
			path:    exePath,
			version: string(versionOut),
		})
	}
	return jvms
}

func runHotpatch(j *jvm) error {
	version, ok := findVersion(j)
	if !ok {
		logger.Printf("Unsupported Java version %q", version)
		return nil
	}
	var options []string
	switch version {
	case java17:
		patchPath := filepath.Join(hotdog.HotdogContainerDir, hotdog.HotdogJDK17Patch)
		options = append(jvmOpts, "-cp", patchPath, "-DfatJar="+patchPath, hotdog.HotdogJDK17Class, strconv.Itoa(j.pid))
	case java11:
		patchPath := filepath.Join(hotdog.HotdogContainerDir, hotdog.HotdogJDK11Patch)
		options = append(jvmOpts, "-cp", patchPath, hotdog.HotdogJDK11Class, strconv.Itoa(j.pid))
	case java8:
		// Sometimes java is invoked as $JAVAHOME/jre/bin/java versus $JAVAHOME/bin/java, try to correct for this.
		bindir := filepath.Dir(j.path)
		basedir := filepath.Dir(bindir)
		dirname := filepath.Base(basedir)
		if dirname == "jre" {
			basedir = filepath.Dir(basedir)
		}
		patchPath := filepath.Join(hotdog.HotdogContainerDir, hotdog.HotdogJDK8Patch)
		options = append(jvmOpts, "-cp", filepath.Join(basedir, "lib", "tools.jar")+":"+patchPath, hotdog.HotdogJDK8Class, strconv.Itoa(j.pid))
	default:
		return nil
	}
	patch := exec.Command(j.path, options...)
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
