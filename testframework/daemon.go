package testframework

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

type DaemonProcess struct {
	CmdLine []string
	Cmd     *exec.Cmd
	Log     *lockedWriter

	logger    *log.Logger
	isRunning bool
}

func NewDaemonProcess(cmdline []string, prefix string) *DaemonProcess {
	return &DaemonProcess{
		CmdLine: cmdline,
		Log:     &lockedWriter{w: new(strings.Builder)},
		logger:  log.New(os.Stdout, fmt.Sprintf("%s: ", prefix), log.LstdFlags),
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
	d.logger.Printf("starting command %s", cmd.String())

	w := io.MultiWriter(d.Log, &logWriter{l: d.logger, filter: os.Getenv("LIGHTNING_TESTFRAMEWORK_FILTER")})
	cmd.Stdout = w

	errReader, err := cmd.StderrPipe()
	if err != nil {
		d.logger.Println("error creating StderrPipe for cmd", err)
		return
	}

	errScanner := bufio.NewScanner(errReader)
	go func() {
		for errScanner.Scan() {
			d.logger.Printf("stdErr: %s\n", errScanner.Text())
		}
	}()

	err = cmd.Start()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error starting cmd", err)
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

	scanner := bufio.NewScanner(strings.NewReader(d.Log.String()))
	for scanner.Scan() {
		match := rx.Find([]byte(scanner.Text()))
		if match != nil {
			return true, nil
		}
	}
	return false, nil
}

func (d *DaemonProcess) WaitForLog(regex string, timeout time.Duration) error {
	d.logger.Printf("wait for log: %s", regex)
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

type lockedWriter struct {
	sync.RWMutex

	buf []byte
	w   io.Writer
}

func (w *lockedWriter) Write(b []byte) (n int, err error) {
	w.Lock()
	defer w.Unlock()

	w.buf = append(w.buf, b...)
	return w.w.Write(b)
}

func (w *lockedWriter) String() string {
	w.RLock()
	defer w.RUnlock()

	return string(w.buf)
}

type logWriter struct {
	l      *log.Logger
	filter string
}

func (w *logWriter) Write(b []byte) (n int, err error) {
	scanner := bufio.NewScanner(bytes.NewReader(b))
	for scanner.Scan() {
		text := scanner.Text()
		if w.filter == "" || strings.Contains(text, w.filter) {
			w.l.Println(text)
		}
	}
	return len(b), nil
}
