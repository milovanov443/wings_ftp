package ftp

import (
	"fmt"

	"github.com/apex/log"
)

// FTPLogger implements the FTP logger interface.
type FTPLogger struct{}

func (l *FTPLogger) Print(sessionID string, message interface{}) {
	log.WithField("session", sessionID).Debug(fmt.Sprint(message))
}

func (l *FTPLogger) Printf(sessionID string, format string, v ...interface{}) {
	log.WithField("session", sessionID).Debugf(format, v...)
}

func (l *FTPLogger) PrintCommand(sessionID string, command string, params string) {
	log.WithFields(log.Fields{
		"session": sessionID,
		"command": command,
		"params":  params,
	}).Debug("ftp command")
}

func (l *FTPLogger) PrintResponse(sessionID string, code int, message string) {
	log.WithFields(log.Fields{
		"session": sessionID,
		"code":    code,
		"message": message,
	}).Debug("ftp response")
}
