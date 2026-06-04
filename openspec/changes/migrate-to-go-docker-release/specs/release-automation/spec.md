## ADDED Requirements

### Requirement: Pull Request Validation Workflow
The repository SHALL provide a GitHub Actions validation workflow that checks Go formatting, Go tests, Go builds, installer syntax, Docker build viability, and OpenSpec validation for proposed changes.

#### Scenario: Pull request validation passes
- **WHEN** a pull request changes Go code, packaging, installer, documentation, or OpenSpec artifacts
- **THEN** GitHub Actions runs the relevant validation jobs before the change is merged

#### Scenario: Invalid installer syntax is introduced
- **WHEN** a pull request introduces shell syntax errors in the installer
- **THEN** the validation workflow fails before release

### Requirement: Tagged Release Workflow
The repository SHALL provide a tagged release workflow that builds versioned Go binary archives for supported operating systems and architectures.

#### Scenario: Version tag is pushed
- **WHEN** a tag matching the release version pattern is pushed
- **THEN** GitHub Actions builds and uploads versioned binary archives for the configured OS and architecture matrix

### Requirement: Release Checksums
The release workflow SHALL generate checksum files for downloadable release artifacts.

#### Scenario: User verifies a binary
- **WHEN** a user downloads a release binary archive
- **THEN** a checksum file is available from the same release for verification

### Requirement: Container Image Publishing
The release workflow SHALL build and publish multi-platform Linux Docker images to the configured registry with immutable version tags and documented moving tags.

#### Scenario: Release image is published
- **WHEN** the tagged release workflow completes successfully
- **THEN** the registry contains a versioned image tag and any configured moving tags such as `latest`

### Requirement: Release Permissions And Provenance
GitHub Actions workflows SHALL use least-privilege permissions appropriate to validation, release asset upload, and container publishing.

#### Scenario: Workflow token permissions are reviewed
- **WHEN** maintainers inspect the workflow definitions
- **THEN** each workflow declares only the permissions required for its jobs
