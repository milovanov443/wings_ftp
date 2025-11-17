package ftp

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"emperror.dev/errors"
	"github.com/apex/log"
	"github.com/spf13/afero"

	"github.com/pterodactyl/wings/server"
) // NOTE: keep io import for PutFile, use afero.File for Create method

// FTPDriver implements the FTP driver interface.
type FTPDriver struct {
	manager  *server.Manager
	BasePath string
	ReadOnly bool
	user     string
	server   *server.Server // Cache server to avoid repeated lookups
}

// getServer retrieves the server for the current user.
func (driver *FTPDriver) getServer() (*server.Server, error) {
	// Return cached server if available
	if driver.server != nil {
		return driver.server, nil
	}

	if driver.user == "" {
		return nil, errors.New("no user set")
	}

	// Usernames follow the format: user_{server-id}
	validUsernameRegexp := regexp.MustCompile(`^(?i)(.+)_([a-z0-9]{8}|[a-z0-9-]{36})$`)
	
	if !validUsernameRegexp.MatchString(driver.user) {
		return nil, errors.New("invalid username format")
	}

	// Extract server ID from username
	parts := strings.Split(driver.user, "_")
	if len(parts) < 2 {
		return nil, errors.New("invalid username format")
	}

	serverKey := parts[len(parts)-1]

	// Find the server - try by UUID first, then by short ID
	s := driver.manager.Find(func(srv *server.Server) bool {
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
		return nil, errors.New("server not found")
	}

	// Cache the server
	driver.server = s
	return s, nil
}

// ChangeDir changes the current directory.
func (driver *FTPDriver) ChangeDir(path string) error {
	_, err := driver.getServer()
	if err != nil {
		return err
	}
	return nil
}

// Stat returns file information.
func (driver *FTPDriver) Stat(path string) (os.FileInfo, error) {
	s, err := driver.getServer()
	if err != nil {
		return nil, err
	}

	realPath := driver.buildPath(s, path)
	return os.Stat(realPath)
}

// ListDir lists directory contents.
func (driver *FTPDriver) ListDir(path string) ([]os.FileInfo, error) {
	s, err := driver.getServer()
	if err != nil {
		return nil, err
	}

	realPath := driver.buildPath(s, path)

	entries, err := os.ReadDir(realPath)
	if err != nil {
		return nil, err
	}

	var files []os.FileInfo
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, info)
	}

	return files, nil
}

// DeleteDir deletes a directory.
func (driver *FTPDriver) DeleteDir(path string) error {
	if driver.ReadOnly {
		return errors.New("read-only server")
	}

	s, err := driver.getServer()
	if err != nil {
		return err
	}

	realPath := driver.buildPath(s, path)
	return os.RemoveAll(realPath)
}

// DeleteFile deletes a file.
func (driver *FTPDriver) DeleteFile(path string) error {
	if driver.ReadOnly {
		return errors.New("read-only server")
	}

	s, err := driver.getServer()
	if err != nil {
		return err
	}

	realPath := driver.buildPath(s, path)
	return os.Remove(realPath)
}

// Rename renames a file or directory.
func (driver *FTPDriver) Rename(fromPath, toPath string) error {
	if driver.ReadOnly {
		return errors.New("read-only server")
	}

	s, err := driver.getServer()
	if err != nil {
		return err
	}

	from := driver.buildPath(s, fromPath)
	to := driver.buildPath(s, toPath)

	return os.Rename(from, to)
}

// MakeDir creates a directory.
func (driver *FTPDriver) MakeDir(path string) error {
	if driver.ReadOnly {
		return errors.New("read-only server")
	}

	s, err := driver.getServer()
	if err != nil {
		return err
	}

	realPath := driver.buildPath(s, path)
	return os.MkdirAll(realPath, 0755)
}

// GetFile retrieves a file for reading.
func (driver *FTPDriver) GetFile(path string, offset int64) (int64, io.ReadCloser, error) {
	s, err := driver.getServer()
	if err != nil {
		return 0, nil, err
	}

	realPath := driver.buildPath(s, path)

	f, err := os.Open(realPath)
	if err != nil {
		return 0, nil, err
	}

	info, err := f.Stat()
	if err != nil {
		f.Close()
		return 0, nil, err
	}

	if offset > 0 {
		_, err = f.Seek(offset, io.SeekStart)
		if err != nil {
			f.Close()
			return 0, nil, err
		}
	}

	return info.Size(), f, nil
}

// PutFile stores a file.
func (driver *FTPDriver) PutFile(path string, data io.Reader, offset int64) (int64, error) {
	if driver.ReadOnly {
		return 0, errors.New("read-only server")
	}

	s, err := driver.getServer()
	if err != nil {
		return 0, err
	}

	realPath := driver.buildPath(s, path)

	// Create directory if needed
	dir := filepath.Dir(realPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return 0, err
	}

	var f *os.File

	if offset > 0 {
		// Append mode
		f, err = os.OpenFile(realPath, os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			return 0, err
		}
		defer f.Close()

		if _, err = f.Seek(offset, io.SeekStart); err != nil {
			return 0, err
		}
	} else {
		// Create/truncate mode
		f, err = os.Create(realPath)
		if err != nil {
			return 0, err
		}
		defer f.Close()
	}

	bytes, err := io.Copy(f, data)
	if err != nil {
		return 0, err
	}

	return bytes, nil
}

// buildPath constructs the real filesystem path for a server with security checks.
// Prevents directory traversal and symlink attacks.
func (driver *FTPDriver) buildPath(s *server.Server, requestPath string) string {
	// Clean the path to prevent directory traversal
	cleaned := filepath.Clean(requestPath)

	// Remove leading slash
	cleaned = strings.TrimPrefix(cleaned, "/")

	// Build full path: /var/lib/pterodactyl/volumes/{uuid}/{path}
	serverRoot := filepath.Join(driver.BasePath, s.ID())
	fullPath := filepath.Join(serverRoot, cleaned)

	// Security check 1: Ensure the resulting path is within the server root
	// This prevents ../../../ attacks
	absServerRoot, _ := filepath.Abs(serverRoot)
	absFullPath, _ := filepath.Abs(fullPath)
	
	if !strings.HasPrefix(absFullPath, absServerRoot+string(filepath.Separator)) && absFullPath != absServerRoot {
		log.WithFields(log.Fields{
			"server":       s.ID(),
			"request_path": requestPath,
			"real_path":    fullPath,
			"resolved":     absFullPath,
		}).Warn("FTP path traversal attempt blocked")
		// Return a path that doesn't exist to prevent access
		return filepath.Join(serverRoot, ".blocked")
	}

	// Security check 2: Resolve symlinks and ensure we're still within server root
	// This prevents symlink attacks to access files outside the server directory
	realPath, err := filepath.EvalSymlinks(fullPath)
	if err != nil {
		// File might not exist yet, but we already validated the path
		realPath = fullPath
	}
	
	realPath, _ = filepath.Abs(realPath)
	absServerRoot, _ = filepath.Abs(serverRoot)
	
	if !strings.HasPrefix(realPath, absServerRoot+string(filepath.Separator)) && realPath != absServerRoot {
		log.WithFields(log.Fields{
			"server":       s.ID(),
			"request_path": requestPath,
			"real_path":    realPath,
		}).Warn("FTP symlink attack attempt blocked")
		// Return a path that doesn't exist to prevent access
		return filepath.Join(serverRoot, ".blocked")
	}

	log.WithFields(log.Fields{
		"server":       s.ID(),
		"request_path": requestPath,
		"real_path":    fullPath,
	}).Debug("FTP path mapping")

	return fullPath
}

// ClientDriver implements ftpserver.ClientDriver interface.
type ClientDriver struct {
	*FTPDriver
}

func (cd *ClientDriver) Init(cc interface{}) {
}

func (cd *ClientDriver) ChangeDir(path string) error {
	return cd.FTPDriver.ChangeDir(path)
}

func (cd *ClientDriver) Stat(path string) (os.FileInfo, error) {
	return cd.FTPDriver.Stat(path)
}

func (cd *ClientDriver) ListDir(path string, callback func(os.FileInfo) error) error {
	files, err := cd.FTPDriver.ListDir(path)
	if err != nil {
		return err
	}

	for _, f := range files {
		if err := callback(f); err != nil {
			return err
		}
	}

	return nil
}

func (cd *ClientDriver) DeleteDir(path string) error {
	return cd.FTPDriver.DeleteDir(path)
}

func (cd *ClientDriver) DeleteFile(path string) error {
	return cd.FTPDriver.DeleteFile(path)
}

func (cd *ClientDriver) Rename(from, to string) error {
	return cd.FTPDriver.Rename(from, to)
}

// MakeDir retained for backward naming, Mkdir added per interface.
func (cd *ClientDriver) MakeDir(path string) error { return cd.FTPDriver.MakeDir(path) }
func (cd *ClientDriver) Mkdir(path string, mode os.FileMode) error { return cd.FTPDriver.MakeDir(path) }
func (cd *ClientDriver) MkdirAll(path string, mode os.FileMode) error { return cd.FTPDriver.MakeDir(path) }

func (cd *ClientDriver) GetFile(path string, offset int64) (int64, io.ReadCloser, error) {
	return cd.FTPDriver.GetFile(path, offset)
}

func (cd *ClientDriver) PutFile(path string, data io.Reader, offset int64) (int64, error) {
	return cd.FTPDriver.PutFile(path, data, offset)
}

func (cd *ClientDriver) Chmod(path string, mode os.FileMode) error {
	// Not implemented
	return nil
}

func (cd *ClientDriver) Chown(path string, uid, gid int) error {
	// Not implemented
	return nil
}

func (cd *ClientDriver) Chtimes(path string, atime, mtime time.Time) error {
	// Not implemented
	return nil
}

func (cd *ClientDriver) Create(path string) (afero.File, error) {
	if cd.FTPDriver.ReadOnly {
		return nil, errors.New("read-only server")
	}
	// Resolve server
	s, err := cd.FTPDriver.getServer()
	if err != nil {
		return nil, err
	}
	realPath := cd.FTPDriver.buildPath(s, path)
	// Ensure parent dirs
	if err := os.MkdirAll(filepath.Dir(realPath), 0755); err != nil {
		return nil, err
	}
	f, err := os.Create(realPath)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (cd *ClientDriver) Name() string {
	return "pterodactyl-ftp"
}

func (cd *ClientDriver) Open(path string) (afero.File, error) {
	s, err := cd.FTPDriver.getServer()
	if err != nil {
		return nil, err
	}
	realPath := cd.FTPDriver.buildPath(s, path)
	return os.Open(realPath)
}

func (cd *ClientDriver) OpenFile(path string, flag int, mode os.FileMode) (afero.File, error) {
	s, err := cd.FTPDriver.getServer()
	if err != nil {
		return nil, err
	}
	realPath := cd.FTPDriver.buildPath(s, path)
	return os.OpenFile(realPath, flag, mode)
}

func (cd *ClientDriver) Remove(path string) error {
	if cd.FTPDriver.ReadOnly {
		return errors.New("read-only server")
	}
	s, err := cd.FTPDriver.getServer()
	if err != nil {
		return err
	}
	realPath := cd.FTPDriver.buildPath(s, path)
	return os.Remove(realPath)
}

func (cd *ClientDriver) RemoveAll(path string) error {
	if cd.FTPDriver.ReadOnly {
		return errors.New("read-only server")
	}
	s, err := cd.FTPDriver.getServer()
	if err != nil {
		return err
	}
	realPath := cd.FTPDriver.buildPath(s, path)
	return os.RemoveAll(realPath)
}
