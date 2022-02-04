package process

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	seccompStatus  = "Seccomp:"
	seccompFilters = "Seccomp_filters:"
	uid            = "Uid:"
	gid            = "Gid:"
)

// ProcessStatus represents a process status
type ProcessStatus struct {
	State   int
	Filters int
	Uid     int
	Gid     int
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
	var filtersLine, stateLine, uidLine, gidLine string

	for s.Scan() {
		text := s.Text()
		switch {
		case strings.Contains(text, seccompStatus):
			stateLine = strings.TrimSpace(text[len(seccompStatus):])
		case strings.Contains(text, seccompFilters):
			filtersLine = strings.TrimSpace(text[len(seccompFilters):])
		case strings.Contains(text, uid):
			uidLine = text[len(uid):]
		case strings.Contains(text, gid):
			gidLine = text[len(gid):]
		}
		if filtersLine != "" && stateLine != "" && uidLine != "" && gidLine != "" {
			break
		}
	}

	state, err := strconv.Atoi(stateLine)
	if err != nil {
		return nil, err
	}
	filters, err := strconv.Atoi(filtersLine)
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

	return &ProcessStatus{State: state, Filters: filters, Uid: puid, Gid: pgid}, nil
}

// parseIdLine parses "Id"-like lines, read from the file
// `/proc/<pid>/status`
func parseIdLine(line string) (int, error) {
	line = strings.TrimSpace(line)
	str := strings.SplitN(line, "\t", 2)[0]
	return strconv.Atoi(str)
}
