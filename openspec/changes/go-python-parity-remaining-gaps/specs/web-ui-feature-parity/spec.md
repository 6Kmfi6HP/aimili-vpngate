## ADDED Requirements

### Requirement: Node Search And Filtering
The management UI SHALL provide search and filter capabilities for the node list.

#### Scenario: Text search across multiple fields
- **WHEN** the user types in the search input
- **THEN** the UI SHALL filter nodes by matching against country, location, IP, ASN, and ISP fields (case-insensitive)

#### Scenario: Country filter dropdown
- **WHEN** the user opens the country filter dropdown
- **THEN** it SHALL show all countries present in the node list with node counts, and selecting one filters the table

#### Scenario: Combined search and country filter
- **WHEN** both search text and country filter are active
- **THEN** the UI SHALL apply both filters simultaneously (AND logic)

### Requirement: Node List Pagination
The management UI SHALL paginate the node list.

#### Scenario: Page navigation
- **WHEN** more than 11 nodes are available
- **THEN** the UI SHALL show pagination controls (first/prev/next/last) with a page indicator and total item count

#### Scenario: Pagination preserves filters
- **WHEN** the user changes pages while search or country filter is active
- **THEN** the filters SHALL remain applied across pages

### Requirement: Auto-Refresh And Polling
The management UI SHALL automatically refresh data at appropriate intervals.

#### Scenario: Background auto-refresh
- **WHEN** the management page is visible and the user is idle
- **THEN** the UI SHALL poll `/api/nodes` every 10 seconds and re-render the node table

#### Scenario: Connection status polling
- **WHEN** the user initiates a connection
- **THEN** the UI SHALL poll `/api/nodes` every 1 second until the connection succeeds or fails, showing an animated connecting state

#### Scenario: Gateway status polling
- **WHEN** the gateway status modal is open
- **THEN** the UI SHALL poll `/api/gateway_status` every 3 seconds

#### Scenario: Log polling
- **WHEN** the logs modal is open
- **THEN** the UI SHALL poll `/api/logs` every 2.5 seconds

### Requirement: Visual Feedback
The management UI SHALL provide visual indicators for node quality and connection state.

#### Scenario: Latency color coding
- **WHEN** a node's latency is displayed
- **THEN** the UI SHALL color it green (<50ms), yellow (<150ms), or red (>=150ms)

#### Scenario: Active connection card
- **WHEN** a VPN connection is active or connecting
- **THEN** the UI SHALL show a dedicated card with node details, animated connecting state, latency, and disconnect button

#### Scenario: Node sorting
- **WHEN** nodes are rendered
- **THEN** the UI SHALL sort them by score (descending) then by ID for stable ordering

### Requirement: Localization
The management UI SHALL display Chinese translations for metadata values.

#### Scenario: Country name translation
- **WHEN** a country name is displayed
- **THEN** the UI SHALL show the Chinese translation (e.g., "Japan" -> "日本", "United States" -> "美国")

#### Scenario: Status/Quality/IPType translation
- **WHEN** probe status, network quality, or IP type values are displayed
- **THEN** the UI SHALL translate them to Chinese labels

### Requirement: Log Management
The management UI SHALL provide log export and filtering capabilities.

#### Scenario: Log level filtering
- **WHEN** the user selects a log category (All/Proxy/VPN/System)
- **THEN** the UI SHALL filter displayed log entries by the selected module

#### Scenario: Log export
- **WHEN** the user clicks the export button
- **THEN** the UI SHALL download the current log content as a .txt file

#### Scenario: Log copy
- **WHEN** the user clicks the copy button
- **THEN** the UI SHALL copy the log content to the clipboard

### Requirement: Connection Safety
The management UI SHALL require confirmation for destructive actions.

#### Scenario: Disconnect confirmation
- **WHEN** the user clicks the disconnect button
- **THEN** the UI SHALL show a confirmation dialog before disconnecting
