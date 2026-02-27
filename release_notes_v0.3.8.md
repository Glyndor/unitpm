# 🦁 Lynx v0.3.8 Release Notes

This release focuses on **Security Hardening** and **Zero-Config Isolation** improvements.

## 🛡️ Security & Isolation
- **Secure-by-Default Polkit Rules**: The Debian package now automatically installs a restrictive Polkit rule that allows the unprivileged `lynx` user to manage **only** systemd units starting with `lynx-`. This enables `--isolation dynamic` to work out-of-the-box without manual configuration, while preventing any potential privilege escalation to critical system services.
- **DynamicUser Hardening**: Enhanced the `systemd-run` execution profile for dynamic isolation.

## 📦 Packaging
- **Conflict Resolution**: Renamed package to `lynx-pm` to avoid conflicts with the `lynx` text browser package in Debian/Ubuntu repositories.
- **Path Standardization**: Moved persistent state to `/var/lib/lynx-pm` and logs to `/var/log/lynx-pm` to comply with FHS standards.

## 🐛 Fixes
- Fixed potential race conditions in socket permission application during startup.
- Improved error messages when running in unsupported environments (non-Linux).

## 🚀 Upgrade Instructions
```bash
# Download the new package
curl -L -o lynxd.deb https://github.com/Jaro-c/Lynx/releases/latest/download/lynx-pm_0.3.8-1_amd64.deb

# Install (will automatically replace the old version)
sudo apt install ./lynxd.deb
```
