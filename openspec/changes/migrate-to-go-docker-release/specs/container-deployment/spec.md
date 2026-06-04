## ADDED Requirements

### Requirement: Docker Image Runtime
The system SHALL provide a Docker image containing the Go AimiliVPN binary, OpenVPN, certificate roots, and required Linux networking tools needed to run the service in a container.

#### Scenario: Container starts with required privileges
- **WHEN** the image is run with `/dev/net/tun`, required Linux capabilities, persistent data storage, and configured ports
- **THEN** the container starts AimiliVPN and reports service status through logs and the management UI

#### Scenario: Container is missing tun access
- **WHEN** the image is run without `/dev/net/tun` or equivalent tun access
- **THEN** the container reports an actionable startup diagnostic

### Requirement: Secure Compose Defaults
The system SHALL provide Docker Compose configuration that keeps management and proxy ports bound to the host loopback interface by default while allowing explicit opt-in public exposure.

#### Scenario: Default Compose deployment is local-only
- **WHEN** a user starts the default Compose file
- **THEN** published management and proxy ports are reachable from the Docker host loopback address and are not published on all host interfaces

#### Scenario: Public exposure requires opt-in
- **WHEN** a user chooses a public exposure Compose example or override
- **THEN** the configuration clearly changes host bindings and documents the management UI and proxy exposure risk

### Requirement: Persistent Container Data
The Compose deployment SHALL persist AimiliVPN data, authentication, logs, state, node cache, and generated OpenVPN configs outside the container filesystem.

#### Scenario: Container is recreated
- **WHEN** the container is removed and recreated with the same volume or bind mount
- **THEN** user configuration, authentication, and service state are preserved

### Requirement: Minimal Container Privileges
The Docker documentation and Compose files SHALL prefer targeted device and capability configuration over fully privileged containers.

#### Scenario: Operator reviews privileges
- **WHEN** an operator reads or runs the default Compose configuration
- **THEN** the required device mounts and capabilities are visible and privileged mode is not the default

### Requirement: Multi-Platform Linux Images
Release automation SHALL build and publish Docker images for supported Linux platforms with at least `linux/amd64` and `linux/arm64` coverage.

#### Scenario: Release publishes container platforms
- **WHEN** a tagged release workflow completes successfully
- **THEN** the published image manifest includes the supported Linux platforms documented for that release
