package seccomp

import (
	"fmt"
	"syscall"
	"unsafe"

	"github.com/bottlerocket/hotdog/process"
	"golang.org/x/sys/unix"
)

/// GetSeccompFilter reads the seccomp filters of the passed pid
func GetSeccompFilter(targetPID int) (allFilters [][]syscall.SockFilter, err error) {
	status, err := process.ParseProcessStatus(targetPID)
	if err != nil {
		return nil, err
	}
	// There isn't a filter to copy if the State isn't SECCOMP_MODE_FILTER
	if status.State != unix.SECCOMP_MODE_FILTER {
		return nil, nil
	}
	// Attach to the process via `PTRACE_SEIZE`, which doesn't stop
	// the process right away unlike `PTRACE_ATTACH`
	if err := ptraceSeize(targetPID); err != nil {
		return nil, fmt.Errorf("got error while seizing process %d: %v", targetPID, err)
	}
	// Send the stop signal to traced process
	if err := ptraceInterrupt(targetPID); err != nil {
		return nil, fmt.Errorf("got error while interrupting process: %v", err)
	}
	// Wait for the process to change state
	var waitStatus syscall.WaitStatus
	_, err = syscall.Wait4(targetPID, &waitStatus, 0, nil)
	if err != nil {
		return nil, fmt.Errorf("got error while waiting for process: %v", err)
	}
	// Fail if the target process isn't stopped
	if !waitStatus.Stopped() {
		return nil, fmt.Errorf("expected process '%d' to be stopped")
	}

	// Only attempt to detach if the target process was interrupted
	defer func() {
		detachErr := syscall.PtraceDetach(targetPID)
		if err == nil && detachErr != nil {
			err = fmt.Errorf("Failed to detach: %v", detachErr)
		}
	}()

	for i := 0; ; i++ {
		// Get the filter size
		sz, err := ptraceSeccompGetFilter(targetPID, i, nil)
		if err != nil {
			// An ENOENT error means we are at the end of the filters list
			if err == unix.ENOENT {
				break
			}
			// Fail in any other error
			return nil, fmt.Errorf("got error while sizing the filter: %v", err)
		}
		// Get the filter data
		seccompFilter := make([]syscall.SockFilter, sz)
		_, err = ptraceSeccompGetFilter(targetPID, i, &seccompFilter[0])
		if err != nil {
			return nil, fmt.Errorf("got error while getting the filter data: %v", err)
		}
		allFilters = append(allFilters, seccompFilter)
	}

	return allFilters, err
}

// SetSeccompFilters sets the seccomp filters for the running process
func SetSeccompFilters(filters [][]syscall.SockFilter) error {
	// Filters have to be set in reverse order, so the most recently installed
	// filter is executed first:
	// https://man7.org/linux/man-pages/man2/seccomp.2.html
	sockFprogs := buildSockFprogs(filters)
	for i := len(sockFprogs) - 1; i >= 0; i-- {
		if err := unix.Prctl(unix.PR_SET_SECCOMP, unix.SECCOMP_MODE_FILTER, uintptr(unsafe.Pointer(&sockFprogs[i])), 0, 0); err != nil {
			return err
		}
	}
	return nil
}

// buildSockFprogs returns an array of syscall.SockFprog objects, from an
// array of arrays of syscall.SockFilter
func buildSockFprogs(filters [][]syscall.SockFilter) []syscall.SockFprog {
	progs := make([]syscall.SockFprog, len(filters))
	for i := 0; i < len(filters); i++ {
		progs[i].Len = uint16(len(filters[i]))
		progs[i].Filter = &filters[i][0]
	}
	return progs
}

// ptraceSeccompGetFilter retrieves the seccomp filters for the passed PID. The 'filter'
// parameter is a pointer to the first element in an array of `syscall.SockFilter` objects.
// https://man7.org/linux/man-pages/man2/ptrace.2.html
func ptraceSeccompGetFilter(pid int, addr int, filter *syscall.SockFilter) (int, error) {
	var filterPtr uintptr
	if filter == nil {
		filterPtr = 0
	} else {
		filterPtr = uintptr(unsafe.Pointer(filter))
	}

	sz, err := ptrace(unix.PTRACE_SECCOMP_GET_FILTER, pid, uintptr(addr), filterPtr)
	if err != nil {
		return 0, err
	}
	return int(sz), nil
}

// ptraceSeize attaches to the process specified in 'pid'. Unlike 'syscall.PtraceAttach',
// ptraceSeize doesn't stop the process.
// https://man7.org/linux/man-pages/man2/ptrace.2.html
func ptraceSeize(pid int) error {
	_, err := ptrace(unix.PTRACE_SEIZE, pid, uintptr(0), uintptr(unix.PTRACE_O_TRACESYSGOOD))
	return err
}

// ptraceInterrupt stops the process specified in 'pid'.
// https://man7.org/linux/man-pages/man2/ptrace.2.html
func ptraceInterrupt(pid int) error {
	_, err := ptrace(unix.PTRACE_INTERRUPT, pid, uintptr(0), uintptr(0))
	return err
}

// Golang doesn't export a raw 'ptrace' syscall
func ptrace(request int, pid int, addr uintptr, data uintptr) (int, error) {
	sz, _, e1 := syscall.Syscall6(unix.SYS_PTRACE, uintptr(request), uintptr(pid), uintptr(addr), data, 0, 0)
	if e1 != 0 {
		return 0, e1
	}
	return int(sz), nil
}
