package ftp

import (
	"emperror.dev/errors"
	"github.com/apex/log"

	"github.com/pterodactyl/wings/server"
)

// FTPAuth implements the FTP authentication interface.
type FTPAuth struct {
	manager *server.Manager
}

// CheckPasswd validates FTP credentials - not used with ftpserverlib.
func (auth *FTPAuth) CheckPasswd(username, password string) (bool, error) {
	log.WithFields(log.Fields{
		"username": username,
	}).Debug("FTP authentication attempt (deprecated method)")
	return false, errors.New("use ftpserverlib AuthUser instead")
}
