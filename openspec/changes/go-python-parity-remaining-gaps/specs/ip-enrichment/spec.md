## ADDED Requirements

### Requirement: IP Metadata Enrichment
The Go runtime SHALL enrich VPNGate node data with IP metadata from an external API and display it in the management UI.

#### Scenario: Nodes are enriched after fetch
- **WHEN** nodes are fetched from the VPNGate API via `RefreshNodes()`
- **THEN** the runtime SHALL batch-query `ip-api.com/batch` for uncached IPs and populate `Owner`, `ASN`, `ASName`, `Location`, `IPType`, and `Quality` fields on each node

#### Scenario: Enrichment results are cached
- **WHEN** an IP has been queried within the last 7 days
- **THEN** the runtime SHALL use the cached result instead of making a new API call

#### Scenario: Enrichment is opt-out
- **WHEN** `AIMILIVPN_IP_ENRICHMENT` is set to `false`
- **THEN** the runtime SHALL skip enrichment entirely and leave node metadata fields empty

#### Scenario: Enrichment API is unavailable
- **WHEN** the `ip-api.com/batch` request fails
- **THEN** the runtime SHALL log the failure, use any available cached results, and continue without blocking node operations

#### Scenario: Batch processing handles large node sets
- **WHEN** more than 100 nodes need enrichment
- **THEN** the runtime SHALL process them in chunks of 100 IPs per API request

#### Scenario: Enrichment cache persists across restarts
- **WHEN** the service restarts
- **THEN** previously cached enrichment results SHALL be loaded from `ip_cache.json` in the data directory

### Requirement: IP Enrichment UI Display
The management UI SHALL display IP metadata columns when enrichment data is available.

#### Scenario: Node table shows enrichment columns
- **WHEN** the user opens the management UI
- **THEN** the node table SHALL include columns for ASN, ISP/Owner, Location, IP Type, and Network Quality

#### Scenario: Enrichment fields are translated
- **WHEN** IP Type or Quality values are displayed
- **THEN** the UI SHALL translate codes to user-friendly Chinese labels (e.g., "hosting" -> "数据中心", "residential" -> "住宅")
