package router

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/apex/log"
	"github.com/gin-gonic/gin"
)

type ftpChangePasswordRequest struct {
	Username        string `json:"username" binding:"required"`
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password" binding:"required"`
}

// postFtpChangePassword handles changing FTP password for a user.
// POST /api/servers/:server/ftp/change-password
// Request body: {username, current_password, new_password}
func postFtpChangePassword(c *gin.Context) {
	s := ExtractServer(c)
	
	var req ftpChangePasswordRequest
	if err := c.BindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request body. Required fields: username, new_password",
		})
		return
	}

	// Validate input
	if len(req.NewPassword) == 0 {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": "New password cannot be empty",
		})
		return
	}

	if len(req.NewPassword) < 6 {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": "New password must be at least 6 characters long",
		})
		return
	}

	logger := log.WithFields(log.Fields{
		"subsystem": "ftp",
		"server_id": s.ID(),
		"username":  req.Username,
	})

	// Check if password file exists
	passwordDir := "/var/lib/pterodactyl/passwords"
	passwordFile := filepath.Join(passwordDir, req.Username+".txt")
	
	_, err := os.Stat(passwordFile)
	fileExists := err == nil

	// If file exists, verify current password (if provided)
	if fileExists {
		// If current password is provided, verify it
		if len(req.CurrentPassword) > 0 {
			if !verifyFtpPassword(req.Username, req.CurrentPassword) {
				logger.Warn("FTP password change failed: invalid current password")
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"error": "Current password is incorrect",
				})
				return
			}
		}
		// If no current password provided but file exists, just allow the change
		// (Panel may not always provide current password on first setup)
	} else {
		logger.Info("FTP password file does not exist, creating new one")
	}

	// Change password
	if err := changeFtpPassword(req.Username, req.NewPassword); err != nil {
		logger.WithField("error", err).Error("failed to change FTP password")
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to change password: " + err.Error(),
		})
		return
	}

	logger.Info("FTP password changed successfully")
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Password changed successfully",
	})
}

// verifyFtpPassword checks if the password is correct for the FTP user.
func verifyFtpPassword(username, password string) bool {
	passwordDir := "/var/lib/pterodactyl/passwords"
	passwordFile := filepath.Join(passwordDir, username+".txt")

	data, err := os.ReadFile(passwordFile)
	if err != nil {
		return false
	}

	storedPassword := strings.TrimSpace(string(data))
	return storedPassword == password
}

// changeFtpPassword updates the FTP password for a user.
func changeFtpPassword(username, newPassword string) error {
	passwordDir := "/var/lib/pterodactyl/passwords"
	passwordFile := filepath.Join(passwordDir, username+".txt")

	// Ensure directory exists
	if err := os.MkdirAll(passwordDir, 0700); err != nil {
		return err
	}

	// Write new password to file with restrictive permissions
	if err := os.WriteFile(passwordFile, []byte(newPassword), 0600); err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"subsystem": "ftp",
		"username":  username,
		"file":      passwordFile,
	}).Debug("FTP password file updated")

	return nil
}
