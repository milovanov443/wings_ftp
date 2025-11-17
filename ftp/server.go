package ftp

import (
	"context"
	"crypto/tls"
	stderrors "errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"emperror.dev/errors"
	"github.com/apex/log"
	ftpserver "github.com/fclairamb/ftpserverlib"

	"github.com/pterodactyl/wings/config"
	"github.com/pterodactyl/wings/remote"
	"github.com/pterodactyl/wings/server"
)

//goland:noinspection GoNameStartsWithPackageName
type FTPServer struct {
	manager  *server.Manager
	BasePath string
	ReadOnly bool
	Listen   string
	server   *ftpserver.FtpServer
	client   remote.Client
}

func New(m *server.Manager, client remote.Client) *FTPServer {
	cfg := config.Get().System
	ftpCfg := cfg.Ftp
	return &FTPServer{
		manager:  m,
		client:   client,
		BasePath: cfg.Data,
		ReadOnly: ftpCfg.ReadOnly,
		Listen:   ftpCfg.Address + ":" + strconv.Itoa(ftpCfg.Port),
	}
}

// Run starts the FTP server and adds a persistent listener to handle inbound
// FTP connections.
func (c *FTPServer) Run() error {
	ftpServer := ftpserver.NewFtpServer(&FTPServerDriver{
		manager:  c.manager,
		client:   c.client,
		basePath: c.BasePath,
		readOnly: c.ReadOnly,
		listen:   c.Listen,
	})

	c.server = ftpServer

	log.WithField("listen", c.Listen).Info("starting FTP server")

	if err := ftpServer.ListenAndServe(); err != nil {
		log.WithField("error", err).Error("FTP server error")
		return err
	}

	return nil
}

// Shutdown gracefully stops the FTP server.
func (c *FTPServer) Shutdown(ctx context.Context) error {
	if c.server != nil {
		return c.server.Stop()
	}
	return nil
}

// FTPServerDriver implements ftpserver.MainDriver interface.
type FTPServerDriver struct {
	manager  *server.Manager
	client   remote.Client
	basePath string
	readOnly bool
	listen   string
}

func (d *FTPServerDriver) GetSettings() (*ftpserver.Settings, error) {
	return &ftpserver.Settings{
		ListenAddr:               d.listen,
		PublicHost:               "",
		PassiveTransferPortRange: &ftpserver.PortRange{Start: 40000, End: 50000},
		DisableMLSD:              false,
		DisableMLST:              false,
		Banner:                   "Pterodactyl FTP Server",
	}, nil
}

func (d *FTPServerDriver) ClientConnected(cc ftpserver.ClientContext) (string, error) {
	log.WithField("remote_addr", cc.RemoteAddr()).Debug("FTP client connected")
	return "Welcome to Pterodactyl FTP Server", nil
}

func (d *FTPServerDriver) ClientDisconnected(cc ftpserver.ClientContext) {
	log.WithField("remote_addr", cc.RemoteAddr()).Debug("FTP client disconnected")
}

func (d *FTPServerDriver) AuthUser(cc ftpserver.ClientContext, username, password string) (ftpserver.ClientDriver, error) {
	// Usernames follow the format: user_{server-id}
	// Validate format first
	validUsernameRegexp := regexp.MustCompile(`^(?i)(.+)_([a-z0-9]{8}|[a-z0-9-]{36})$`)
	
	if !validUsernameRegexp.MatchString(username) {
		log.WithFields(log.Fields{
			"username": username,
			"ip":       cc.RemoteAddr().String(),
		}).Warn("failed to validate FTP credentials: invalid username format")
		return nil, errors.New("invalid username format")
	}

	parts := strings.Split(username, "_")
	if len(parts) < 2 {
		log.WithField("username", username).Warn("failed to validate FTP credentials: invalid username format")
		return nil, errors.New("invalid username format")
	}

	// Last part is server key, everything before is user
	serverKey := parts[len(parts)-1]

	// Find the server
	var s *server.Server
	s = d.manager.Find(func(srv *server.Server) bool {
		srvID := srv.ID()
		// Try exact match (full UUID)
		if srvID == serverKey {
			return true
		}
		// Try short ID match (first 8 chars)
		if len(srvID) >= 8 && srvID[:8] == serverKey {
			return true
		}
		// Try last 8 chars match
		if len(srvID) >= 8 && strings.HasSuffix(srvID, serverKey) {
			return true
		}
		return false
	})

	if s == nil {
		log.WithFields(log.Fields{
			"username":   username,
			"server_key": serverKey,
			"ip":         cc.RemoteAddr().String(),
		}).Warn("failed to validate FTP credentials: server not found")
		return nil, errors.New("server not found")
	}

	// Verify password against /etc/passwd
	logger := log.WithFields(log.Fields{
		"subsystem": "ftp",
		"username":  username,
		"ip":        cc.RemoteAddr().String(),
	})
	logger.Debug("validating FTP credentials against password file")

	if !verifyPassword(username, password) {
		logger.Warn("failed to validate FTP credentials (invalid password)")
		return nil, errors.New("invalid password")
	}

	// Extract actual username from full username (without server id)
	actualUser := strings.Join(parts[:len(parts)-1], "_")
	
	// Security check: Verify user has access to the server
	// Load server ACL from config or database
	if !userHasAccessToServer(actualUser, s.ID()) {
		log.WithFields(log.Fields{
			"username":  username,
			"server_id": s.ID(),
			"ip":        cc.RemoteAddr().String(),
		}).Warn("FTP access denied: user does not have permission for this server")
		return nil, errors.New("access denied: you do not have permission to access this server")
	}

	// Return client driver
	return &ClientDriver{
		FTPDriver: &FTPDriver{
			manager:  d.manager,
			BasePath: d.basePath,
			ReadOnly: d.readOnly,
			user:     username,
			server:   s, // Cache the server to avoid repeated lookups
		},
	}, nil
}

// userHasAccessToServer checks if a user has permission to access a specific server.
// For now, we allow access if the password file exists (implicit permission).
// In future, this could check an ACL database or Panel API.
func userHasAccessToServer(username, serverID string) bool {
	// Security: Check if password file exists for this user_serverid combination
	// This implicitly means the user has been granted access
	passwordDir := "/var/lib/pterodactyl/passwords"
	fullUsername := username + "_" + serverID[:8]
	passwordFile := filepath.Join(passwordDir, fullUsername+".txt")
	
	_, err := os.Stat(passwordFile)
	if err != nil {
		log.WithFields(log.Fields{
			"username": username,
			"server_id": serverID,
		}).Debug("FTP access denied: no password file found for user_server combination")
		return false
	}
	
	return true
}

// verifyPassword checks if the password is correct by reading from file
// Reads from /var/lib/pterodactyl/passwords/{username}.txt
func verifyPassword(username, password string) bool {
	passwordDir := "/var/lib/pterodactyl/passwords"
	passwordFile := filepath.Join(passwordDir, username+".txt")
	
	log.WithFields(log.Fields{
		"username": username,
		"password_file": passwordFile,
	}).Debug("verifyPassword called")
	
	// Read password from file
	data, err := os.ReadFile(passwordFile)
	if err != nil {
		log.WithFields(log.Fields{
			"username": username,
			"error": err,
		}).Warn("failed to read password file")
		return false
	}

	storedPassword := strings.TrimSpace(string(data))
	
	// Compare passwords
	matches := storedPassword == password
	log.WithFields(log.Fields{
		"username": username,
		"match": matches,
	}).Debug("password comparison result")
	
	return matches
}

func (d *FTPServerDriver) GetTLSConfig() (*tls.Config, error) {
	// Return error to disable TLS - plain FTP only
	return nil, stderrors.New("TLS not configured")
}
