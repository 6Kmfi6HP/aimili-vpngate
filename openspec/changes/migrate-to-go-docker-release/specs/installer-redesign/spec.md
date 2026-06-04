## ADDED Requirements

### Requirement: Native Install Mode
The installer SHALL support a native Linux install mode that installs or verifies required system packages, installs the Go AimiliVPN binary, creates service configuration, initializes persistent data, and preserves the management shortcut.

#### Scenario: Fresh native install
- **WHEN** a user runs the installer in native mode on a supported Linux distribution
- **THEN** the installer installs AimiliVPN as a managed service and prints the management URL and lifecycle command summary

#### Scenario: Existing native install is upgraded
- **WHEN** a user runs the installer against an existing native installation
- **THEN** the installer upgrades the binary and service assets while preserving persistent data and user authentication

### Requirement: Docker Install Mode
The installer SHALL support a Docker deployment mode that installs or writes Compose configuration, prepares persistent data storage, validates Docker availability, and starts the Compose service when requested.

#### Scenario: Fresh Docker install
- **WHEN** a user selects Docker mode on a host with Docker and Compose support
- **THEN** the installer writes the Compose deployment, creates or reuses persistent storage, and starts the AimiliVPN container

#### Scenario: Docker is unavailable
- **WHEN** Docker mode is selected on a host without Docker support
- **THEN** the installer either offers an explicit Docker installation path or exits with clear official-install guidance

### Requirement: Lifecycle Commands
The installer and management command SHALL provide install, upgrade, status, logs, restart, stop, start, configuration, and uninstall flows for supported deployment modes.

#### Scenario: User checks service status
- **WHEN** a user runs the lifecycle status command after native or Docker installation
- **THEN** the command reports service state, management UI location, proxy port, and active VPN status where available

#### Scenario: User views logs
- **WHEN** a user runs the lifecycle logs command
- **THEN** the command streams or prints AimiliVPN logs for the selected deployment mode

### Requirement: Safe Uninstall
The uninstall flow SHALL distinguish service removal from persistent data deletion and SHALL require explicit confirmation before deleting user data.

#### Scenario: User uninstalls service only
- **WHEN** a user runs uninstall and declines data deletion
- **THEN** service assets are removed while persistent data remains on disk

### Requirement: Distro And Service Manager Compatibility
Native installer behavior SHALL support systemd and OpenRC where practical and SHALL detect unsupported service managers with clear manual guidance.

#### Scenario: OpenRC host is detected
- **WHEN** the installer runs on an OpenRC-based supported distribution
- **THEN** it writes OpenRC service configuration instead of systemd units

### Requirement: Bilingual Installer Documentation
The redesigned installer SHALL have bilingual user-facing documentation for native install, Docker install, Compose, upgrade, rollback, logs, uninstall, and migration from Python releases.

#### Scenario: User reads install docs
- **WHEN** a user opens the README or install documentation
- **THEN** Chinese and English instructions are available for the supported install modes
