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
)

// ProcessStatus represents a process status
type ProcessStatus struct {
	State   int
	Filters int
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
	var filtersLine, stateLine string

	for s.Scan() {
		text := s.Text()
		switch {
		case strings.Contains(text, seccompStatus):
			stateLine = strings.TrimSpace(text[len(seccompStatus):])
		case strings.Contains(text, seccompFilters):
			filtersLine = strings.TrimSpace(text[len(seccompFilters):])
		}
		if filtersLine != "" && stateLine != "" {
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

	return &ProcessStatus{State: state, Filters: filters}, nil
}
