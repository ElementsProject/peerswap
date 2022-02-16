package log

import (
	"log"
)

var (
	logger PeerswapLogger
)

type PeerswapLogger interface {
	Infof(format string, v ...interface{})
	Debugf(format string, v ...interface{})
}

func SetLogger(peerswapLogger PeerswapLogger) {
	logger = peerswapLogger
}

func Infof(format string, v ...interface{}) {
	if logger != nil {
		logger.Infof(format, v...)
	} else {
		log.Printf("[INFO] "+format, v...)
	}
}

func Debugf(format string, v ...interface{}) {
	if logger != nil {
		logger.Debugf(format, v...)
	} else {
		log.Printf("[DEBUG] "+format, v...)
	}
}
