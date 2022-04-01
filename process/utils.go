package process

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

const (
	seccompStatus = "Seccomp:"
	uid           = "Uid:"
	gid           = "Gid:"
)

// ProcessStatus represents a process status
type ProcessStatus struct {
	State int
	Uid   int
	Gid   int
}

// ParseProcessStatus reads the '/proc/<pid>/status' file to retrieve
// information about the target PID
func ParseProcessStatus(targetPID int) (*ProcessStatus, error) {
	f, err := os.OpenFile(filepath.Join("/proc", strconv.Itoa(targetPID), "status"), os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	var stateLine, uidLine, gidLine string

	for s.Scan() {
		text := s.Text()
		switch {
		case strings.Contains(text, seccompStatus):
			stateLine = strings.TrimSpace(text[len(seccompStatus):])
		case strings.Contains(text, uid):
			uidLine = text[len(uid):]
		case strings.Contains(text, gid):
			gidLine = text[len(gid):]
		}
		if stateLine != "" && uidLine != "" && gidLine != "" {
			break
		}
	}

	state, err := strconv.Atoi(stateLine)
	if err != nil {
		return nil, err
	}
	puid, err := parseIdLine(uidLine)
	if err != nil {
		return nil, err
	}
	pgid, err := parseIdLine(gidLine)
	if err != nil {
		return nil, err
	}

	return &ProcessStatus{State: state, Uid: puid, Gid: pgid}, nil
}

// parseIdLine parses "Id"-like lines, read from the file
// `/proc/<pid>/status`
func parseIdLine(line string) (int, error) {
	line = strings.TrimSpace(line)
	str := strings.SplitN(line, "\t", 2)[0]
	return strconv.Atoi(str)
}

// ConstrainFileDescriptors sets the FD_CLOEXEC flag in all
// the open file descriptors of the current process
func ConstrainFileDescriptors() error {
	pid := strconv.Itoa(os.Getpid())
	files, err := ioutil.ReadDir("/proc/self/fd")
	if err != nil {
		return fmt.Errorf("failed to read /proc for %s", pid)
	}
	for _, file := range files {
		fd, err := strconv.Atoi(file.Name())
		if err != nil {
			return fmt.Errorf("failed to transform file name: %v", file.Name())
		}
		_, err = unix.FcntlInt(uintptr(fd), unix.F_SETFD, unix.FD_CLOEXEC)
		// `fcntl` returns `EBADF` when the file descriptor is no longer open,
		// so we can silently ignore such errors
		if err != nil && err != unix.EBADF {
			return fmt.Errorf("failed to set FD_CLOEXEC in '%d': %v", fd, err)
		}
	}
	return nil
}
