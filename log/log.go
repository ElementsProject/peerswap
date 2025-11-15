package log

import (
	"log"
)

var (
	logger PeerswapLogger
)

type PeerswapLogger interface {
	Infof(format string, v ...any)
	Debugf(format string, v ...any)
}

func SetLogger(peerswapLogger PeerswapLogger) {
	logger = peerswapLogger
}

func Infof(format string, v ...any) {
	if logger != nil {
		logger.Infof(format, v...)
	} else {
		log.Printf("[INFO] "+format, v...)
	}
}

func Debugf(format string, v ...any) {
	if logger != nil {
		logger.Debugf(format, v...)
	} else {
		log.Printf("[DEBUG] "+format, v...)
	}
}

type logType int

const (
	DEBUG logType = 1
	INFO          = 2
)

type typeLogger struct {
	typ logType
}

func (t *typeLogger) Write(p []byte) (n int, err error) {
	switch t.typ {
	case DEBUG:
		Debugf(string(p))
	case INFO:
		Infof(string(p))
	}
	return len(p), nil
}

func NewDebugLogger() *typeLogger {
	return &typeLogger{typ: DEBUG}
}

func NewInfoLogger() *typeLogger {
	return &typeLogger{typ: INFO}
}
