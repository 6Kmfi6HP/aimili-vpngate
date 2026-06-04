## ADDED Requirements

### Requirement: Log Rotation
The Go runtime SHALL automatically clean up old log files to prevent unbounded disk growth.

#### Scenario: Old log files are deleted
- **WHEN** the maintenance loop runs and more than 1 hour has elapsed since the last cleanup
- **THEN** the runtime SHALL scan the `logs/` directory and delete any `YYYY-MM-DD.json` files older than 3 days

#### Scenario: Cleanup is rate-limited
- **WHEN** the maintenance loop runs more frequently than hourly
- **THEN** log cleanup SHALL NOT run again until at least 3600 seconds have elapsed since the last cleanup

#### Scenario: Cleanup handles parse errors gracefully
- **WHEN** a log filename cannot be parsed as a date
- **THEN** the runtime SHALL fall back to the file's modification time for age comparison

### Requirement: OpenVPN Version Detection
The Go runtime SHALL detect the installed OpenVPN version and adapt its command-line flags accordingly.

#### Scenario: Version is detected on first use
- **WHEN** OpenVPN is first invoked
- **THEN** the runtime SHALL run `openvpn --version`, parse the version number from the output, and cache the result

#### Scenario: Cipher flags adapt to version
- **WHEN** building the OpenVPN command
- **THEN** the runtime SHALL use `data-ciphers` for OpenVPN >=2.5 and `ncp-ciphers` for versions <2.5

#### Scenario: Version detection failure uses safe default
- **WHEN** `openvpn --version` fails or returns unparseable output
- **THEN** the runtime SHALL default to version 2.4 behavior

### Requirement: Orphan Process Cleanup
The Go runtime SHALL clean up stale OpenVPN processes from prior runs on startup.

#### Scenario: Startup kills orphaned processes
- **WHEN** the service starts via `Run()`
- **THEN** the runtime SHALL kill any existing OpenVPN processes matching `openvpn.*tun0` or `openvpn.*vpngate_data` patterns

#### Scenario: Cleanup uses pkill
- **WHEN** orphan cleanup is triggered
- **THEN** the runtime SHALL use `pkill -f` with the appropriate patterns and log the results

### Requirement: Policy Routing Retry
The Go runtime SHALL retry policy routing setup to handle transient failures.

#### Scenario: Route setup retries on failure
- **WHEN** `setupPolicyRouting()` fails to execute `ip route add` or `ip rule add`
- **THEN** the runtime SHALL retry up to 3 times with a 1-second delay between attempts

#### Scenario: All retries exhausted
- **WHEN** all 3 route setup attempts fail
- **THEN** the runtime SHALL log the failure and return the last error

### Requirement: Uptime Tracking
The Go runtime SHALL track server uptime and use it for heartbeat grace periods.

#### Scenario: Start time is recorded
- **WHEN** the application is initialized via `New()`
- **THEN** the current time SHALL be recorded as the server start time

#### Scenario: Heartbeat checks respect startup grace period
- **WHEN** the server has been running for less than 15 seconds
- **THEN** heartbeat status checks SHALL return "starting" regardless of heartbeat timestamps

#### Scenario: Grace period expires
- **WHEN** the server has been running for more than 35 seconds
- **THEN** heartbeat status checks SHALL use normal stale-detection logic
