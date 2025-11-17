wings модуль для pterodactyl для локаций, но без SFTP, но с FTP

SFTP вырезан

Теперь тут FTP


Логин формата admin_00af504b (не admin.00af504b как у pterodactyl)

Пароль хранится в /var/lib/pterodactyl/passwords/login.txt - без шифрования

[FTPService] Full URL: https://LOCATION_PATH:8080/api/servers/{server_uuid}/ftp/change-password

[FTPService] Payload: {"username":"{username}","current_password":"{current_password}","new_password":"{new_password}"}

При создании сервера pterodactyl не отправляет никаких запросов, FTP по умолчанию недоступен, нужно самостоятельно послать запрос на этот эндпоинт и создать пароль 

При создании сервера, юзера нет, поэтому current_password указываем пустой, а new_password - как новый пароль


Успех:
[FTPService] Could not fetch SFTP data from Pterodactyl: The route api/client/servers/8e633f08-47d3-4048-a3df-530f2ec3963f/sftp could not be found.

[FTPService] Full URL: https://loc.azerta.ru:8080/api/servers/8e633f08-47d3-4048-a3df-530f2ec3963f/ftp/change-password

[FTPService] Payload: {"username":"admin_8e633f08","current_password":"","new_password":"S6VU1dtv3jnvwKGr"}

[FTPService] Response status: 200

[FTPService] Response data: {"message":"Password changed successfully","success":true}


Модуль будет удобен тем, кто делает биллинг+pterodactyl систему, в идеале через оболочку, как это сделает azerta.ru в январе 2026 (или декабря 2025)