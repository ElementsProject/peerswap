package testframework

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

type DaemonProcess struct {
	CmdLine []string
	Cmd     *exec.Cmd
	StdOut  *lockedWriter
	StdErr  *lockedWriter

	prefix    string
	isRunning bool
}

func NewDaemonProcess(cmdline []string, prefix string) *DaemonProcess {
	return &DaemonProcess{
		CmdLine: cmdline,
		StdOut:  &lockedWriter{w: new(strings.Builder), prefix: []byte(fmt.Sprintf("%s: ", prefix))},
		StdErr:  &lockedWriter{w: new(strings.Builder), prefix: []byte(fmt.Sprintf("%s: ", prefix))},
		prefix:  prefix,
	}
}

func (d *DaemonProcess) AppendCmdLine(options []string) {
	if options != nil {
		d.CmdLine = append(d.CmdLine, options...)
	}
}

func (d *DaemonProcess) WithCmd(cmd string) {
	if len(d.CmdLine) > 0 {
		cmdLine := []string{cmd}
		d.CmdLine = append(cmdLine, d.CmdLine[1:]...)
		return
	}
	d.CmdLine = []string{cmd}
}

func (d *DaemonProcess) Run() {
	cmd := exec.Command(d.CmdLine[0], d.CmdLine[1:]...)
	d.Cmd = cmd
	cmd.Stdout = d.StdOut
	cmd.Stderr = d.StdErr

	err := cmd.Start()
	if err != nil {
		fmt.Fprintln(d.StdErr, "error starting cmd", err)
		return
	}

	d.isRunning = true
}

func (d *DaemonProcess) Kill() {
	if d.isRunning {
		d.Cmd.Process.Kill()
	}
}

func (d *DaemonProcess) HasLog(regex string) (bool, error) {
	rx, err := regexp.Compile(regex)
	if err != nil {
		return false, fmt.Errorf("Compile(regex) %w", err)
	}

	scanner := bufio.NewScanner(strings.NewReader(d.StdOut.String()))
	for scanner.Scan() {
		match := rx.Find([]byte(scanner.Text()))
		if match != nil {
			return true, nil
		}
	}
	return false, nil
}

func (d *DaemonProcess) WaitForLog(regex string, timeout time.Duration) error {
	// d.logger.Printf("wait for log: %s", regex)
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			return fmt.Errorf("timeout reached while waiting for `%s` in logs", regex)
		default:
			ok, err := d.HasLog(regex)
			if err != nil {
				return fmt.Errorf("HasLog() %w", err)
			}
			if ok {
				return nil
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func (d *DaemonProcess) Prefix() string {
	return d.prefix
}

type lockedWriter struct {
	sync.RWMutex

	prefix []byte
	buf    []byte
	w      io.Writer
}

func (w *lockedWriter) Write(b []byte) (n int, err error) {
	w.Lock()
	defer w.Unlock()
	w.buf = append(w.buf, w.prefix...)
	w.buf = append(w.buf, b...)
	return w.w.Write(b)
}

func (w *lockedWriter) String() string {
	w.RLock()
	defer w.RUnlock()

	return string(w.buf)
}

func (w *lockedWriter) Filter(regex string) []byte {
	w.RLock()
	defer w.RUnlock()

	rx, err := regexp.Compile(regex)
	if err != nil {
		return nil
	}

	var buf []byte
	scanner := bufio.NewScanner(bytes.NewReader(w.buf))
	for scanner.Scan() {
		match := rx.Find(scanner.Bytes())
		if match != nil {
			buf = append(buf, scanner.Bytes()...)
			buf = append(buf, []byte("\n")...)
		}
	}
	return buf
}

func (w *lockedWriter) Tail(n int, regex string) string {
	w.RLock()
	defer w.RUnlock()

	rx, err := regexp.Compile(regex)
	if err != nil {
		return ""
	}

	var lines []string
	scanner := bufio.NewScanner(bytes.NewReader(w.buf))
	for scanner.Scan() {
		match := rx.Find(scanner.Bytes())
		if match != nil {
			lines = append(lines, scanner.Text())
		}
	}

	// We want to have the possibility to print out the whole log.
	if n < 1 || n > len(lines) {
		n = len(lines)
	}

	if n > 0 && n <= len(lines) {
		return strings.Join(lines[len(lines)-n:], "\n")
	}
	return strings.Join(lines, "\n")
}
