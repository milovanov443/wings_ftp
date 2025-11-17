# Pterodactyl Wings FTP Module

This module adds FTP support to Pterodactyl Wings alongside the existing SFTP functionality.

## Features

- **Parallel operation**: FTP runs on port 21, SFTP on port 2022
- **Same authentication**: Uses Panel API for credential validation
- **Same permissions**: Access to the same server files as SFTP
- **Passive mode**: Supports passive FTP (ports 40000-50000)
- **Read-only mode**: Optional read-only access

## Files Structure

```
ftp/
├── server.go      - Main FTP server implementation
├── driver.go      - File operations (read, write, delete, etc.)
├── auth.go        - Authentication via Panel API
└── logger.go      - Logging integration
```

## How It Works

### 1. Authentication
- Username format: `user.serverid` (e.g., `admin.abcd1234`)
- Password: Panel user password
- Validates via Panel API: `/api/remote/sftp/auth`

### 2. File Access
- Files stored at: `/var/lib/pterodactyl/volumes/{server_uuid}/`
- Same permissions as SFTP
- Owner: `pterodactyl:pterodactyl`
- FTP user access via ACL

### 3. Operations Supported
- **LIST**: Directory listing
- **RETR**: Download files
- **STOR**: Upload files
- **DELE**: Delete files
- **RMD**: Remove directories
- **MKD**: Create directories
- **RNFR/RNTO**: Rename files/directories

## Configuration

Add to `/etc/pterodactyl/config.yml`:

```yaml
system:
  ftp:
    bind_address: 0.0.0.0
    bind_port: 21
    read_only: false
```

## Dependencies

Uses `goftp.io/server/v2` for FTP server implementation:

```bash
go get goftp.io/server/v2
```

## Security Considerations

- **Unencrypted**: FTP transmits credentials in plain text
- **Recommendation**: Use SFTP for production or implement FTPS (TLS)
- **Firewall**: Ensure only necessary ports are open (21, 40000-50000)

## Future Improvements

- [ ] Add FTPS (FTP over TLS) support
- [ ] Per-user bandwidth limits
- [ ] Connection limits
- [ ] IP whitelist/blacklist
- [ ] More detailed logging

## Testing

```bash
# Connect with command-line FTP client
ftp panel.azerta.ru 21
# Username: admin.abcd1234
# Password: your_password

# Or use FileZilla
# Protocol: FTP
# Host: panel.azerta.ru
# Port: 21
# Username: admin.abcd1234
# Password: your_password
```

## Troubleshooting

### Connection refused
- Check if Wings is running: `systemctl status wings`
- Check if port 21 is open: `netstat -tulpn | grep :21`
- Check firewall: `ufw status`

### Authentication failed
- Verify credentials in Panel
- Check Wings logs: `journalctl -u wings -f | grep ftp`
- Ensure Panel API is accessible

### Cannot list directory
- Check file permissions: `ls -la /var/lib/pterodactyl/volumes/`
- Verify ACL: `getfacl /var/lib/pterodactyl/volumes/{uuid}/`

### Passive mode doesn't work
- Ensure ports 40000-50000 are open
- Check `PassivePorts` setting in code
- Verify NAT/routing if behind firewall
