# Changelog

All notable changes to RouteBox are documented here.

## [0.16.1] - 2026-05-09

### Improvements

**Frontend:**
- **Breakdown — apex-domain rollup in BY DOMAIN panel** — subdomains collapse to their effective TLD+1 via the public suffix list (`tldts`), so e.g. all `*.googlevideo.com` rows aggregate under a single `googlevideo.com` entry in both live and historical modes. IPs and the `-` placeholder are unaffected.
- **Breakdown — drill into subdomains** — clicking an apex row zooms the BY DOMAIN panel into the subdomains under that apex; clicking a subdomain drills further to a single host. The chip in the filter bar steps back out.
- **Breakdown — Reset history action** — destructive button in the page header that opens a confirmation modal and wipes the accumulated traffic store; live counters are unaffected and accumulation resumes from the next sample tick.
- **Breakdown — Show all / Show less toggle moved above the row list** — always visible regardless of scroll position; previously you had to scroll past thousands of rows to collapse a panel.
- **Breakdown — friendly client names in BY SOURCE pie legend** — the donut legend now resolves IPs to their assigned client names (matching the rows below the chart).
- **Domain Sets — sans-serif font for domain rows** — matches the URL typography on Rule Sets.
- **Logs — newest entries at top** — incoming log lines now prepend; auto-scroll snaps to the head of the feed.

**Backend:**
- **Apex domain match in `QueryAggregate`** — the `domain` filter on `GET /api/traffic/history` now expands to the supplied apex plus all of its subdomains (the UI sends apex strings).
- **`POST /api/traffic/reset`** — wipes the `traffic_minute` SQLite table. The sampler's in-memory state is intentionally preserved so deltas keep being attributed correctly from the next tick.
- New helper `traffic.ApexDomain` (backed by `golang.org/x/net/publicsuffix`) and matching JS helper `apexDomain` (backed by `tldts`).

---

## [0.16.0] - 2026-04-26

### New Features

**Frontend:**
- **Clients page** (`/config/clients`) — auto-discovers LAN devices from active connections, lets you assign friendly names and notes inline. Names appear in Connections (group headers + source cells), Dashboard Top Connections, and Breakdown BY SOURCE.
- **Traffic history on Breakdown** — new time range selector (Live / 1h / 3h / 24h / Week / Month). Historical mode reads from a SQLite store maintained by a 60s sampler; filter clicks drill into the same SQL query.
- **Pie charts on Breakdown** — each panel (BY SOURCE / BY DOMAIN / BY CHAIN) gets a donut chart with top-3 + "other" legend. Click a slice to filter the dimension.
- **Domain Sets reworked as inline rule-set editor** — no more local `.json` source / `.srs` compile pipeline. Edits go straight into `route.rule_set[]` of the sing-box config and apply on the next config Apply.

### Improvements

**Frontend:**
- **Dashboard metric bars reordered** — sing-box version + config path now sits above Managed-by / PID / Uptime / Connections.
- **Logs persist last selected level** — `localStorage` survives reloads.
- **Connections — show source IP when grouped by chain** — inside chain groups, the otherwise-redundant Chain column reuses the slot for the source IP (with `whitespace-nowrap` so IPv6 doesn't wrap).
- **Rule Sets form** — Local type removed (was deprecated by Inline). Remote and Inline buttons now equal-width, matching the Format selector. Legacy `type:local` rule-sets render read-only with a `(legacy)` badge and the Edit button hidden in both list and view-modal.
- **Breakdown polish** — single-select per dimension (was multi-set), each panel collapses to top 10 with a Show all (N) toggle that resets on filter change, domain rows show their primary destination IP underneath with `(+N)` for additional resolved IPs (lex-smallest pinned for stability across WS ticks), totals + filter chips split into a stable two-row bar that no longer jumps when filters appear.

**Backend:**
- **Inline rule-set CRUD** — `domains_handlers.go` rewritten as a thin layer over `route.rule_set` of `type: inline`. Validator now accepts empty `rules` arrays (needed for create + last-domain-removed). Handlers deep-copy rule-sets on read so failed updates don't leave the in-memory config in a partial state.
- **Compile pipeline removed** — no more `autoCompileDomainSets` pre-Apply hook, no `.srs` generation, dead `backend/internal/domains` package deleted.
- **Clients manager** (`backend/internal/clients`) — TOML persistence next to settings file, atomic write with `f.Sync()` before rename for crash safety, 30s background save loop.
- **Traffic store** (`backend/internal/traffic`) — `modernc.org/sqlite` (pure-Go, no CGO) keeps minute-bucketed `(source, domain, chain) → upload/download` aggregates with a 35-day retention sweep. Sampler polls Clash `/connections` every 60s, computes byte deltas with counter-reset detection, upserts via `ON CONFLICT DO UPDATE`.
- **HTTP API additions** — `GET/PUT/DELETE /api/clients` for client names, `GET /api/traffic/history?range=...&source=...&domain=...&chain=...` for historical aggregates.

### Notes

- Go directive bumped to `1.25.0` by `modernc.org/sqlite@latest`.
- Go test coverage added: `backend/internal/clients` (8 tests, including TOML round-trip and no-op-when-not-dirty), `backend/internal/traffic` (10 tests covering store schema, query filters, prune, and sampler delta math including counter-reset and closed-connection eviction).

---

## [0.15.1] - 2026-04-21

### New Features

**Frontend:**
- **Traffic Breakdown page** — new `/monitor/breakdown` with 3-way live drill-down. Three panels (Sources, Domains, Chains) aggregate open connections by bytes; clicking any row toggles it as a filter chip and the other two panels re-aggregate. Filters combine as AND across dimensions / OR within a dimension.
- **Group by chain on Connections** — new checkbox that groups rows by their proxy-chain path (e.g. `vless-auto → vless-de`), mutually exclusive with Group by client.
- **Persisted view preferences** — Live updates, Group by client and Group by chain toggles now survive navigating away and back (localStorage).

### Improvements

**Frontend:**
- **Simpler connectivity icon colors** — endpoint/outbound/proxy icon-badges are now binary: green = alive (or untested), red = explicit failure (delay === 0). No more grey "unknown" or orange "slow" states that caused icons to flash red on page entry before the auto-latency test completed.
- **Unified dashboard typography** — PID, uptime, connections, traffic rate, total transfer and config-path values now use the same sans-serif as domain rows (removed `font-mono`/`font-semibold`).

**Backend:**
- **Friendly errors for missing local `.srs` files** — Apply now pre-checks every `route.rule_set` of type `local` and returns a clear per-rule-set message (missing file / directory / stat error) before sing-box sees the config.
- **Stripped ANSI from sing-box stderr** — when sing-box check does surface an error, the toast now shows readable text instead of `\x1b[31mFATAL\x1b[0m...` escape sequences.

---

## [0.15.0] - 2026-03-31

### New Features

**Frontend:**
- **Endpoint connectivity status** — icon-badges on Endpoints page now colored by latency from Clash API (green < 300ms, orange 300-1000ms, red for timeout, gray if untested)
- **Proxy connectivity status** — ProxyCard icons colored by latency; selector/urltest groups show status of active member
- **Source IP in dashboard Top-5** — added sourceIP column to top connections table on the dashboard

### Bug Fixes

**Frontend:**
- **Speed unit no longer affects traffic volume** — bytes/bits setting now only changes speed display (MB/s vs Mbps), traffic totals always show in bytes (KB, MB, GB)
- **Proxy delay reads last history entry** — fixed ProxyCard using first history entry instead of most recent

---

## [0.14.2] - 2026-03-27

### Bug Fixes

**Frontend:**
- **Fixed connections filter/sort** — `$derived(() => ...)` replaced with `$derived.by(() => ...)` so filter input and sort headers actually trigger reactivity
- **Fixed dashboard traffic not resetting after restart** — streams and totals now reset when sing-box is restarted via dashboard button

### Improvements

**Frontend:**
- **Dashboard layout redesign** — replaced 4 info cards (PID, Uptime, Managed by, Connections) with a compact horizontal metrics bar; config path and version merged into a secondary inline bar
- **Connections grouped and collapsed by default** — `groupBySource` defaults to true with groups collapsed, reducing visual noise
- **Fixed connections table header alignment** — sort buttons now fill full cell width for proper column alignment
- **Dashboard top connections badge positioning** — chain badges shifted left closer to hostname

---

## [0.14.1] - 2026-03-20

### Bug Fixes

**Frontend:**
- **Fixed endpoint address CIDR normalization** — addresses without prefix (e.g. `10.8.0.2`) are now auto-appended with `/32` (IPv4) or `/128` (IPv6) to match sing-box `netip.Prefix` requirement
- **Fixed connections table column alignment** — headers and data now properly aligned using `table-fixed` layout with explicit column widths
- **Fixed dashboard mobile layout** — traffic stats stacked vertically, responsive info grid, smaller fonts on small screens, chain chips hidden on mobile

### Improvements

**Frontend:**
- **Speed unit support (bits/bytes)** — the bytes/bits toggle in App Settings now actually works everywhere: traffic page, dashboard, connections. Shared `formatBytes`/`formatSpeed` replaces 4 duplicated local functions
- **Connections grouping by client** — new "Group by client" checkbox groups connections by source IP, showing per-client totals and collapsible groups
- **Removed `defaultNetworkStrategy` from route rule form** — was incorrectly placed in per-rule form
- **Removed Visual Builder** — module removed entirely
- **Quick Links mobile layout** — cards switch to vertical (icon-on-top) layout on small screens

---

## [0.13.5] - 2026-02-25

### Bug Fixes

**Frontend:**
- **Fixed TLS Fragment config format** — sing-box expects `tls_fragment: true` (boolean), not a nested object with `size`/`sleep` fields that don't exist in sing-box. Removed phantom `size` and `sleep` inputs. Only `tls_fragment_fallback_delay` is configurable as a separate duration field.

**Backend:**
- Removed obsolete `validateTlsFragment()` / `validateTlsRecordFragment()` validators that expected object format

### Improvements

**Frontend:**
- **Hide deprecated fields on sing-box 1.12+** — `domain_strategy` on DNS servers and `default_domain_strategy` in route settings are hidden when their replacements (`domain_resolver` / `default_domain_resolver`) are available
- **DNS Settings UX** — "DNS Server" renamed to "Final DNS Server" with tooltip explaining fallback behavior; "Strategy" renamed to "Default Strategy" with hint text clarifying it's the global default for all DNS queries

---

## [0.13.0] - 2026-01-30

### New Features

**Frontend:**
- **Visual Builder Phase 2** - Complete visual routing editor
  - Rule ordering with numbered nodes and reorder buttons (↑↓)
  - Badges for rule_set, inbound filters, and logical rules
  - "Default" badge on final outbound
  - Context menu (right-click) for creating/deleting rules
  - **DNS tab** - Visual editor for DNS rules and servers
    - DNS rule nodes with server connections
    - DNS server nodes with detour badges
    - Side panel for editing DNS rules

**UI Improvements:**
- Collapsible navigation groups (Network, Routing) for cleaner sidebar

### Refactoring

**Frontend:**
- Removed unnecessary `as any` type casts for better type safety
- Typed `toSingboxConfig` and `importConfig` return values properly
- Removed dead `preshared_key` fallback code

---

## [0.12.0] - 2026-01-29

### Refactoring

**Frontend:**
- Major component extraction reducing large file complexity
- OutboundForm.svelte: 1461 → 589 lines (60% reduction)
- EndpointForm.svelte: 885 → 481 lines (46% reduction)
- setup/+page.svelte: 917 → 487 lines (47% reduction)
- config/+page.svelte: 755 → 421 lines (44% reduction)

**New Shared Components:**
- `CollapsibleSection` - Expandable section with arrow toggle
- `ListItemActions` - Hover-reveal edit/delete buttons
- `SelectableCard` - Radio/checkbox card wrapper
- `ProgressIndicator` - Step dots with connecting lines
- `FileDropZone` - Drag-drop textarea combo

**New Setup Components:**
- Extracted wizard steps: InstallStep, VpnConfigStep, UsageModeStep, RuleSetsStep, RoutingModeStep, ApplyStep

**New Config Components:**
- StatsGrid, BackupSection for config overview
- ShadowtlsForm, AnytlsForm, ImportModal for outbound forms
- ObfuscationFields, PeerCard, PeerList, ImportDialog for endpoint forms

---

## [0.11.0] - 2026-01-29

### New Features

**Frontend:**
- **Rule Combiner** - Create logical AND/OR rules by combining multiple condition sets. Toggle between Simple and Combined modes in the rule form.

### Refactoring

**Frontend:**
- Complete decomposition of RuleForm.svelte (920 → 328 lines) into reusable components
- Extracted ActionSelector, OutboundSelector, ConditionsForm, and ActionOptions components

### Removed

**Frontend:**
- Russian localization removed (translation quality needs rework)

---

## [0.10.2] - 2026-01-29

### Fixes

**Frontend:**
- Fixed JSON parse error when closing connections (DELETE requests now handle empty 204 responses)

---

## [0.10.1] - 2026-01-29

### Fixes

**Backend:**
- Fixed 405 Method Not Allowed error when closing connections via Clash API proxy

---

## [0.10.0] - 2026-01-29

### Mobile-First Responsive Design

Major UX overhaul bringing full mobile support to RouteBox.

**Frontend:**
- Responsive mobile sidebar with hamburger menu toggle
- Forms and tables now adapt to mobile screen sizes
- Smooth animations for modal dialogs and toast notifications

### New Features

**Frontend:**
- **Connection Test** in Route Inspector - test connectivity to any destination and see which outbound handles it
- **Rule Templates** expansion - added gaming platforms (Steam, Epic, Discord) and regional categories with full i18n support (EN/RU)
- **HelpTooltip** component - contextual help icons for complex configuration fields
- **domain_resolver** support for outbounds and endpoints (sing-box 1.12+ feature)

### Fixes

**Frontend:**
- Fixed duplicate "sing-box" prefix in version display

---

## [0.9.15] - 2026-01-28

### Technical Debt Reduction

Major refactoring to improve code quality, type safety, and maintainability.

#### Phase 1: Utilities

**Frontend:**
- Created `validators.ts` with 13 validation functions (validatePort, validateRequired, validateTag, validateCIDR, validateBase64Key, validatePortRange, validateDomain, validateUUID, validateURL, validateIP, validateNonEmptyArray, validateOptionalPort, validatePositiveInt)
- Created `errorHandler.ts` with error handling utilities (formatError, handleApiError, safeAsync, tryAsync, logError, createValidationErrors, hasValidationErrors, clearValidationErrors)
- Added generic parsing utilities to `parsers.ts` (parseLines, parseCSV, parsePorts, parseIntArray, parseAddresses, formatLines, formatCSV, parseDuration, parseKeyValuePairs, parsePortRanges, extractDomain, parseReservedBytes)
- Created barrel export `utils/index.ts`
- Refactored EndpointForm to use new validators (proper base64 key validation)
- Refactored OutboundForm to use new validators (proper UUID validation)

#### Phase 2: i18n & Types

**Frontend:**
- Added `validation` i18n keys (30+ messages) for EN and RU locales
- Added `import` i18n keys for proxy link import messages
- Added `TLSConfig`, `MultiplexConfig`, `TransportConfig`, `ObfsConfig` interfaces to types.ts
- Added protocol-specific outbound types: `OutboundVless`, `OutboundHysteria2`, `OutboundShadowsocks`, `OutboundShadowtls`, `OutboundAnytls`, `OutboundSelector`, `OutboundUrltest`
- Added `OutboundTyped` discriminated union for type-safe outbound handling
- Created `typeGuards.ts` with 12 type guard functions (isVlessOutbound, isHysteria2Outbound, isProxyOutbound, supportsTls, etc.)
- Replaced 60+ `(outbound as any)` casts in OutboundForm with proper typed access

#### Phase 3: Component Splitting

**Frontend:**
- Created `components/config/outbound/` directory with modular sub-components:
  - `TlsConfig.svelte` - Shared TLS settings (SNI, fingerprint, Reality)
  - `ServerConfig.svelte` - Shared server/port inputs
  - `VlessForm.svelte` - VLESS protocol form
  - `Hysteria2Form.svelte` - Hysteria2 protocol form
  - `ShadowsocksForm.svelte` - Shadowsocks protocol form
  - `SelectorForm.svelte` - Selector/URLTest form
- Created barrel export `outbound/index.ts`

#### Phase 4: Testing

**Frontend:**
- Added Vitest test infrastructure (`vitest.config.ts`)
- Added test scripts to package.json (`test`, `test:watch`, `test:coverage`)
- Created `validators.test.ts` with 45 tests
- Created `parsers.test.ts` with 57 tests
- Created `typeGuards.test.ts` with 33 tests
- **Total: 135 tests, all passing**

---

## [0.9.14] - 2026-01-28

### App Version Display

**Frontend:**
- Added version display in App Settings page (About section)
- Version is injected at build time via Vite define

---

## [0.9.13] - 2026-01-28

### Endpoints Page Improvements

**Frontend:**
- Added group membership badges on endpoints showing which outbound groups (selector/urltest) use each endpoint
- Endpoints now fetch outbounds data to compute group membership

**Backend:**
- Added validation when creating domain rule sets to prevent tag conflicts with existing rule-sets in sing-box config

---

## [0.9.12] - 2026-01-28

### Outbounds Page Redesign

#### Overview
Redesigned Outbounds page with visual separation between service outbounds (groups) and real outbounds (proxies), plus usage badges showing route rule counts and group membership.

#### Changed

**Frontend:**
- Split outbounds into two sections: "Groups" (selector/urltest) and "Proxies" (direct, vless, etc.)
- Added colored left border and icons per outbound type
- Added "N rules" badge showing how many route rules use each outbound
- Added "→ group1, group2" badge showing which groups contain each proxy
- Service outbounds show member list as compact badges
- Removed "Apply Changes" button (use global status bar instead)
- Added 7 new i18n keys for EN/RU

---

### GeoIP & Settings (2026-01-25)

#### Overview
Added GeoIP support for connection enrichment (country flags, ASN info) and TOML-based application settings.

#### Added

**GeoIP Integration**
- Country flags displayed next to destination IPs in connections table
- Tooltip with country name, ASN, and provider on hover
- GeoIP info in expanded connection details
- Filter connections by country or ASN
- Supports IPInfo and MaxMind MMDB databases

**Application Settings (`routebox.toml`)**
- TOML configuration file with 77 documented options
- Runtime-changeable settings (no restart required):
  - `[geoip]` — enable/disable enrichment, auto-reload
  - `[ui]` — theme, language, speed units, time format
  - `[monitoring]` — enrichment, history limits, intervals
- Restart-required settings clearly marked in comments
- Settings priority: CLI flags > TOML file > auto-detect > defaults

**Settings UI Page**
- New "App Settings" page in Config section
- Sections: GeoIP, User Interface, Monitoring
- Read-only display for restart-required settings
- Save/Reset/Reload functionality

**API Endpoints**
- `GET /api/settings` — get all settings with runtime info
- `PUT /api/settings` — update runtime-safe settings
- `POST /api/settings/reload` — reload from file

#### Files Added
- `routebox.toml` — complete settings template with comments
- `backend/internal/settings/settings.go` — settings manager
- `backend/internal/geoip/geoip.go` — MMDB lookup
- `frontend/src/routes/config/app/+page.svelte` — settings UI

#### Files Modified
- `backend/cmd/routebox/main.go` — added `--settings` flag, settings integration
- `backend/internal/api/handler.go` — settings endpoints, GeoIP enrichment
- `frontend/src/lib/types.ts` — GeoInfo, RouteBoxSettings types
- `frontend/src/lib/api/client.ts` — settings API methods
- `frontend/src/lib/components/monitor/ConnectionTable.svelte` — flags, tooltips
- `frontend/src/routes/+layout.svelte` — "App Settings" sidebar link
- `install.sh` — downloads routebox.toml, creates /etc/routebox/
- `README.md` — GeoIP and settings documentation

---

### Bug Fixes (2026-01-24)

#### Overview
Fixed multiple issues with route rules editing, monitoring, and Clash API integration.

#### Fixed

**1. SNIFF/HIJACK-DNS Rule Forms (CRITICAL)**
- Simplified RuleForm for `sniff` and `hijack-dns` actions per [sing-box documentation](https://sing-box.sagernet.org/configuration/route/rule_action/)
- `sniff` action: Shows only timeout field, no conditions tabs
- `hijack-dns` action: Shows info message, automatically adds `protocol: ['dns']`
- Route/reject actions: Continue showing all condition tabs
- Added `timeout` field to `RouteRule` interface

**2. Logs Not Working**
- Setup wizard now configures log settings: `{ level: 'info', timestamp: true }`
- Previously only configured Clash API but not log settings

**3. Monitor/Proxies/Traffic Errors**
- Created `requestRaw<T>()` function for Clash API endpoints
- Clash API responses are raw JSON (not wrapped in `{ success: true, data: ... }`)
- Previous `request()` function expected wrapper, causing "Unknown error"
- Fixed: `getProxies()`, `getProxy()`, `switchProxy()`, `testLatency()`, `getConnections()`, `closeConnection()`, `closeAllConnections()`

**4. Setup Wizard Routing Rules**
- Added `sniff` rule as first rule (protocol detection)
- `hijack-dns` rule now second (DNS query interception)
- Proper rule order per sing-box best practices

#### Files Modified

**Frontend:**
- `frontend/src/lib/api/client.ts` — Added `requestRaw()`, updated Clash API methods
- `frontend/src/lib/types.ts` — Added `timeout` to `RouteRule`
- `frontend/src/lib/components/config/RuleForm.svelte` — Simplified for sniff/hijack-dns
- `frontend/src/lib/components/config/DraggableRuleList.svelte` — Better descriptions for action rules
- `frontend/src/routes/setup/+page.svelte` — Added log settings, sniff rule

#### References
- [sing-box Rule Actions](https://sing-box.sagernet.org/configuration/route/rule_action/)
- [sing-box Migration Guide](https://sing-box.sagernet.org/migration/)

---

### Unsaved Changes Bar (2026-01-24)

#### Overview
Added a persistent notification bar at the bottom of the screen that appears whenever there are unsaved configuration changes. The bar shows a warning that changes need to be saved, with options to view the list of changes, save them to the running config, or discard them.

#### Added

**Frontend:**
- `unsavedChanges` store (`$lib/stores/unsavedChanges.ts`):
  - `markChanged(section, description)` — Track a change in a section
  - `clearChanges()` — Clear all tracked changes
  - `requestNavigation(path)` — Handle navigation with unsaved changes
  - `cancelNavigation()` / `confirmDiscard()` — Navigation control
  - Tracks changes with section name and description
- `UnsavedChangesBar.svelte` component:
  - Yellow warning bar fixed at bottom of screen (z-50)
  - Shows "You have unsaved changes" message
  - "Show/Hide" toggle to expand list of changes
  - "Save Changes" button — applies config via `api.applyConfig()`
  - "Discard" button — reloads page to discard changes
  - Loading state during save operation
- Integrated into all config pages:
  - **Endpoints** — create, update, delete operations
  - **Outbounds** — create, update, delete operations
  - **Inbounds** — create, update, delete operations
  - **Routes** — rule sets, rules, settings, reorder
  - **DNS** — servers, rules, settings, reorder
  - **Settings** — log and experimental settings
- All pages call `unsavedChanges.clearChanges()` after successful apply

#### Files Modified

**Frontend:**
- `frontend/src/lib/stores/unsavedChanges.ts` — NEW store
- `frontend/src/lib/stores/index.ts` — Export unsavedChanges
- `frontend/src/lib/components/shared/UnsavedChangesBar.svelte` — NEW component
- `frontend/src/routes/+layout.svelte` — Added UnsavedChangesBar
- `frontend/src/routes/config/endpoints/+page.svelte` — markChanged calls
- `frontend/src/routes/config/outbounds/+page.svelte` — markChanged calls
- `frontend/src/routes/config/inbounds/+page.svelte` — markChanged calls
- `frontend/src/routes/config/routes/+page.svelte` — markChanged calls
- `frontend/src/routes/config/dns/+page.svelte` — markChanged calls
- `frontend/src/routes/config/settings/+page.svelte` — markChanged calls

#### UX Notes
- Bar is non-dismissable — user must either save or discard
- Changes are tracked with descriptions like "Created endpoint X", "Deleted rule #3"
- Toast notifications positioned above the bar (bottom-20) to avoid overlap
- Save operation uses the global `api.applyConfig()` endpoint

---

### IPv6 System Detection & Auto-Adaptation (2026-01-24)

#### Overview
Added detection of system-level IPv6 disable state (`net.ipv6.conf.all.disable_ipv6`) and automatic removal of IPv6 addresses from TUN interface configuration when IPv6 is disabled. This prevents sing-box from crashing when attempting to bind IPv6 addresses on systems where IPv6 is completely disabled at the kernel level.

#### Added

**Backend:**
- `IPv6Disabled` field in `SystemChecks` struct (`process/manager.go`)
- Reads `/proc/sys/net/ipv6/conf/all/disable_ipv6` to detect IPv6 state
- `RemoveIPv6FromTunInbounds()` method in config manager (`config/manager.go`):
  - Filters IPv6 addresses from `address` array (modern format)
  - Removes `inet6_address` field (legacy format)
  - Returns `true` if any modifications were made
- `isIPv6Address()` helper function (detects IPv6 by presence of colon)

**API Changes:**
- `Start()` and `Restart()` handlers now:
  - Check `IPv6Disabled` before starting process
  - Automatically remove IPv6 from TUN inbounds if disabled
  - Save modified config to disk
  - Return `warning` field in response when IPv6 was removed
- Response type changed: `{ message: string, warning?: string }`

**Frontend:**
- Added `ipv6_disabled: boolean` to `SystemChecks` interface (`types.ts`)
- Updated `start()` and `restart()` API client types to include optional `warning`
- Dashboard shows IPv6 disabled info in system checks warning block
- New informational banner when all checks pass but IPv6 is disabled:
  - Blue info-style banner explaining IPv6 auto-removal behavior
  - Shows the sysctl setting that triggered it
- `handleStart()` and `handleRestart()` show warning notification when IPv6 removed

#### Files Modified

**Backend:**
- `backend/internal/process/manager.go` — `SystemChecks` struct, `GetSystemChecks()` function
- `backend/internal/config/manager.go` — `RemoveIPv6FromTunInbounds()`, `isIPv6Address()`
- `backend/internal/api/handler.go` — `Start()`, `Restart()` handlers

**Frontend:**
- `frontend/src/lib/types.ts` — `SystemChecks` interface
- `frontend/src/lib/api/client.ts` — `start()`, `restart()` return types
- `frontend/src/routes/+page.svelte` — IPv6 disabled display, warning handling

#### Technical Notes
- `disable_ipv6=1` is different from `ipv6.forwarding=0`:
  - `forwarding=0`: IPv6 works locally but packets aren't routed between interfaces
  - `disable_ipv6=1`: IPv6 is completely disabled at kernel level, `bind()` on IPv6 fails
- sing-box crashes when trying to assign IPv6 address to TUN on systems with disabled IPv6
- Auto-removal is transparent to user but warning notification informs about the action

---

### Sing-box Documentation Compliance (2026-01-24)

#### Overview
Comprehensive update to align RouteBox with official sing-box documentation. Fixes 8 categories of issues across frontend types and backend validation.

#### Changed

**Phase 2a: AWG Peer Field Name (CRITICAL)**
- Renamed `preshared_key` → `pre_shared_key` in `AWGPeer` interface (`types.ts`)
- Updated `EndpointForm.svelte` parser and form handling
- Added `reserved?: number[]` field for WARP support
- Backward compatible: migrates legacy field names on load

**Phase 1: TUN Inbound Address Migration (CRITICAL)**
- Added unified `address: string[]` array (sing-box 1.8+ format) to `Inbound`
- Added `interface_name` and `mtu` fields
- Replaced separate IPv4/IPv6 inputs with comma-separated address field
- Backend validates both new and legacy (`inet4_address`/`inet6_address`) formats
- Files: `types.ts`, `InboundForm.svelte`, `manager.go:validateInbound`

**Phase 6: RuleSet Missing Fields**
- Added `download_detour?: string` for specifying download outbound
- Added `update_interval?: string` (e.g., "24h") for auto-update
- `RuleSetForm.svelte` now accepts `outbounds` prop for detour selection
- New UI fields shown only for remote type rule sets

**Phase 7: DNS Config Missing Fields**
- Added to `DnsSettings` and `DnsConfig`:
  - `cache_capacity?: number` — max cache entries
  - `reverse_mapping?: boolean` — IP→domain mapping
  - `client_subnet?: string` — EDNS Client Subnet
- Updated `/config/dns` page with new settings UI
- Backend `GetDnsSettings`/`UpdateDnsSettings` handle new fields

**Phase 4: URLTest Outbound Fields**
- Added to `Outbound`:
  - `url?: string` — test URL (default: gstatic generate_204)
  - `interval?: string` — test interval (e.g., "3m")
  - `tolerance?: number` — tolerance in ms
  - `idle_timeout?: string` — idle timeout
- `OutboundForm.svelte`: new URLTest settings section (shown when type=urltest)

**Phase 3: Hysteria2 Outbound Fields**
- Added to `Outbound`:
  - `server_ports?: string` — port hopping range (e.g., "1000-2000,3000-4000")
  - `hop_interval?: string` — port hop interval (e.g., "30s")
  - `up_mbps?: number` — upload limit
  - `down_mbps?: number` — download limit
- `OutboundForm.svelte`: new Port Hopping and Bandwidth sections for Hysteria2

**Phase 2b: Endpoint Additional Fields**
- Added to `Endpoint`:
  - `system?: boolean` — use system interface
  - `name?: string` — interface name override
  - `udp_timeout?: string` — UDP timeout (e.g., "5m")
  - `workers?: number` — worker count
- `EndpointForm.svelte`: populated Advanced tab with new fields

**Phase 5: Route Rules Actions & Conditions (MAJOR)**
- Extended `action` type: `'route' | 'reject' | 'sniff' | 'hijack-dns'`
- Made `outbound` optional (required only for `action='route'`)
- Added logical rule support: `type`, `mode`, `rules`, `invert`
- New match conditions:
  - `domain?: string[]` — exact domain match
  - `domain_regex?: string[]` — regex patterns
  - `source_ip_cidr?: string[]` — source IP matching
  - `source_port?: number[]`, `source_port_range?: string[]`
  - `protocol?: string[]` — http, tls, quic, dns, etc.
- `RuleForm.svelte`:
  - Action selector at top with 4 options
  - Outbound selector shown only for 'route' action
  - New "Advanced" tab with regex, source IP/port, protocol
  - Invert checkbox for negating conditions
- Backend `validateRule`: validates action type, outbound conditionally required

#### Files Modified

**Frontend (`frontend/src/lib/`):**
- `types.ts` — All interface updates
- `components/config/EndpointForm.svelte` — pre_shared_key, advanced tab
- `components/config/InboundForm.svelte` — unified address field
- `components/config/OutboundForm.svelte` — URLTest, Hysteria2 fields
- `components/config/RuleForm.svelte` — new actions, advanced conditions
- `components/config/RuleSetForm.svelte` — download_detour, update_interval
- `routes/config/dns/+page.svelte` — new DNS settings
- `routes/config/routes/+page.svelte` — pass outbounds to RuleSetForm

**Backend (`backend/internal/config/`):**
- `manager.go`:
  - `validateInbound` — support new address array
  - `validateRule` — action-based outbound validation
  - `GetDnsSettings`/`UpdateDnsSettings` — new DNS fields

#### Backward Compatibility
- Legacy field names are parsed on load and migrated to new format
- Old configs with `inet4_address`/`inet6_address` still work
- Old configs with `preshared_key` (no underscore) still work

---

### Phase 7: Polish & UX (2026-01-23)

#### Changed

**Frontend:**
- Complete color scheme overhaul in `app.css`:
  - Replaced Catppuccin palette with professional GitHub-inspired colors
  - Dark theme: `#0f1419` base, `#0d1117` mantle, `#161b22` surface
  - Light theme: `#ffffff` base, `#f6f8fa` mantle, `#f0f0f0` surface
  - Muted accent colors for less "toy-like" appearance
  - Better contrast ratios for improved readability
- Fixed text contrast on colored buttons across all pages:
  - Changed `text-[var(--ctp-base)]` to `text-white` on status badges, buttons, and notifications
- Fixed header title color (from accent to neutral text)
- Fixed sidebar section labels with better contrast
- Fixed toast notification text visibility

**Files Modified:**
- `frontend/src/app.css` - Complete rewrite with professional color scheme
- `frontend/src/routes/+layout.svelte` - Header and sidebar contrast fixes
- All page files (`endpoints`, `outbounds`, `inbounds`, `routes`, `dns`, `settings`, `backup`, `import`, `setup`, `traffic`, `logs`, `connections`, `proxies`) - Button text contrast fixes

---

### Phase 6: Import & Wizards (2026-01-23)

#### Added

**Frontend:**
- Link parsers (`$lib/utils/parsers.ts`):
  - `parseVless()` - Parse vless:// URIs with TLS, Reality, WebSocket, gRPC support
  - `parseHysteria2()` - Parse hy2:// and hysteria2:// URIs
  - `parseAWG()` - Parse AmneziaWG/WireGuard config files with obfuscation params
  - `parseConfig()` - Auto-detect and parse any supported format
  - `toSingboxConfig()` - Convert parsed config to sing-box format
- Import page (`/config/import`):
  - Text input for links/configs
  - Live parsing with preview
  - Type detection (VLESS, Hysteria2, AWG)
  - One-click import to config
  - Supported formats guide
- Quick Setup Wizard (`/setup`):
  - Step 1: Import VPN configuration
  - Step 2: Select rule sets (Antizapret, GeoSite RU, GeoIP RU, AdBlock)
  - Step 3: Choose routing mode (Split Tunneling / All Traffic)
  - Step 4: Review and apply configuration
  - Progress indicator
  - Skip option for advanced users
- Added Import link to sidebar navigation

**Documentation:**
- Updated `CHANGELOG.md`

---

### Phase 5 Completion: Advanced Monitoring (2026-01-23)

#### Added

**Frontend:**
- Traffic by Outbound section on Traffic page:
  - Real-time traffic breakdown per outbound
  - Connection count per outbound
  - Visual bar chart comparison
  - Auto-refresh every 5 seconds
- Auto-refresh latency on Proxies page:
  - Toggle for automatic latency testing
  - Configurable interval (15s, 30s, 60s, 2m)
  - Tests all proxies at selected interval

**Note:** Log search was already implemented. Connection filtering by host, chain, rule, and source IP (in expanded view) was already available.

---

### Phase 7: Proxies & Backup (2026-01-23)

#### Added

**Backend:**
- Config Export API (`GET /api/config/export`)
  - Downloads config as `sing-box-config.json` file
  - Uses `Content-Disposition: attachment` header
- Config Import API (`POST /api/config/import`)
  - Validates uploaded JSON config
  - Returns validation results without applying
  - Returns `{ valid, errors, config }` response

**Frontend:**
- `ProxyCard.svelte` component:
  - Displays proxy name, type, and latency
  - Color-coded latency (green <300ms, yellow <600ms, red >600ms)
  - Test latency button with loading state
  - Selector proxy switching dropdown
- New `/monitor/proxies` page:
  - Grid of ProxyCard components
  - Filter by type (all, selectors, endpoints, direct/block)
  - Sort by name or delay
  - "Test All" button for batch latency testing
  - Refresh button
- New `/config/backup` page:
  - Export Configuration section with download button
  - Import Configuration section with drag-and-drop zone
  - File validation with error display
  - Apply imported config button
  - Warning about config replacement
- Added Proxies link to Monitor section in sidebar
- Added Backup link to Config section in sidebar
- API client methods: `exportConfig`, `importConfig`

**Documentation:**
- Created `docs/PHASE7-PROXIES-BACKUP.md` implementation plan
- Updated `CHANGELOG.md`

---

### Phase 6: Monitoring Enhancements (2026-01-23)

#### Added

**Frontend:**
- `ConnectionTable.svelte` - Sortable, filterable connections table:
  - Sort by host, network, upload, download, time
  - Filter by host, chain, or rule
  - Expandable rows with full connection details
  - Close button per connection
- `ProxyCard.svelte` - Proxy display with latency testing:
  - Latency test button with color-coded results
  - Selector proxy switching
  - Delay history display
- New `/monitor/connections` page with:
  - Real-time WebSocket updates (toggleable)
  - Connection stats (count, total upload/download)
  - ConnectionTable component
  - Close All button
- Enhanced Dashboard:
  - Active connections count
  - Total upload/download since start
  - Top 5 connections preview with links
  - 4-column stats grid
- Added Connections link to sidebar navigation
- Added `ConnectionsResponse`, `ProxiesResponse` types
- Added `createConnectionsStream` WebSocket helper
- API client methods: `closeConnection`, `closeAllConnections`, `switchProxy`, `getProxy`

**Documentation:**
- Created `docs/PHASE6-MONITORING.md` implementation plan
- Updated `CHANGELOG.md`

---

### Phase 5: Log & Experimental Settings (2026-01-23)

#### Added

**Backend:**
- Log Settings API (`/api/log`)
  - `GET /` - Get log settings (level, timestamp, output)
  - `PUT /` - Update log settings
- Experimental Settings API (`/api/experimental`)
  - `GET /` - Get experimental settings (cache_file, clash_api)
  - `PUT /` - Update experimental settings
- Validation for log level (trace, debug, info, warn, error, fatal, panic)
- Validation for clash_api default_mode (rule, global, direct)

**Frontend:**
- `LogForm.svelte` - Log settings form:
  - Log level dropdown (trace → panic)
  - Timestamp toggle
  - Output path input
- `ExperimentalForm.svelte` - Experimental settings form:
  - Cache File section (collapsible):
    - Enable toggle
    - Path input
    - Cache ID input
    - Store FakeIP toggle
    - Store RDRC toggle
  - Clash API section (collapsible):
    - External controller input
    - External UI path input
    - UI download URL input
    - Download detour selector
    - Secret input (password field)
    - Default mode selector (rule/global/direct)
- New `/config/settings` page with:
  - Log Settings card
  - Experimental card with collapsible sections
  - Apply Changes button
- Added sidebar navigation for DNS and Settings
- Added `LogSettings` type
- Added `CacheFileSettings`, `ClashApiSettings`, `ExperimentalSettings` types
- API client methods for log and experimental operations

**Documentation:**
- Created `docs/PHASE5-LOG-EXPERIMENTAL.md` implementation plan
- Updated `docs/API.md` with log and experimental endpoints
- Updated `CHANGELOG.md`

---

### Phase 4: DNS Configuration (2026-01-23)

#### Added

**Backend:**
- DNS Servers CRUD API (`/api/dns/servers`)
  - `GET /` - List all DNS servers
  - `POST /` - Create DNS server
  - `PUT /:tag` - Update DNS server
  - `DELETE /:tag` - Delete DNS server (with reference check)
- DNS Rules CRUD API (`/api/dns/rules`)
  - `GET /` - List DNS rules in order
  - `POST /` - Create DNS rule (append)
  - `PUT /:index` - Update DNS rule at index
  - `DELETE /:index` - Delete DNS rule at index
  - `PUT /reorder` - Move rule from one index to another
- DNS Settings API (`/api/dns/settings`)
  - `GET /` - Get DNS settings (strategy, final, cache options)
  - `PUT /` - Update DNS settings
- Validation for DNS servers (tag, type, server address)
- Validation for DNS rules (server reference, rule_set refs)
- Protection against deleting DNS servers referenced by rules or settings

**Frontend:**
- `DnsServerForm.svelte` - Form with presets (Google, Cloudflare, Quad9, AdGuard, Local)
- `DnsRuleForm.svelte` - Multi-tab form for DNS rule conditions:
  - Domains tab: exact, suffix, keyword, regex
  - Other tab: IP CIDR, query types (A, AAAA, etc.)
  - Rule Sets tab: multi-select from available rule sets
- New `/config/dns` page with:
  - DNS settings section (strategy, final server, cache options)
  - DNS servers section with add/edit/delete
  - DNS rules section with drag-and-drop reordering
  - Apply Changes button
- Enhanced `DnsServer` type with all fields
- Enhanced `DnsRule` type with all match conditions
- Added `DnsSettings` type
- API client methods for all DNS operations

**Documentation:**
- Created `docs/PHASE4-DNS.md` implementation plan
- Updated `docs/API.md` with DNS endpoints

---

### Phase 3: Routing & Rule Sets (2026-01-23)

#### Added

**Backend:**
- Route Rule Sets CRUD API (`/api/route/rule-sets`)
  - `GET /` - List all rule sets
  - `POST /` - Create rule set (remote/local)
  - `DELETE /:tag` - Delete rule set (with reference check)
- Route Rules CRUD API (`/api/route/rules`)
  - `GET /` - List rules in order
  - `POST /` - Create rule (append)
  - `PUT /:index` - Update rule at index
  - `DELETE /:index` - Delete rule at index
  - `PUT /reorder` - Move rule from one index to another
- Route Settings API (`/api/route/settings`)
  - `GET /` - Get final outbound and auto_detect_interface
  - `PUT /` - Update settings
- Validation for rule sets (tag, type, url/path)
- Validation for rules (outbound exists, rule_set refs exist)
- Protection against deleting rule sets that are referenced by rules

**Frontend:**
- `RuleSetForm.svelte` - Form with quick-add presets (Antizapret, GeoSite, GeoIP, AdBlock)
- `RuleForm.svelte` - Multi-tab form for rule conditions:
  - Conditions tab: private IP, domains, IPs, ports, network
  - Rule Sets tab: multi-select from available rule sets
  - Process tab: process name/path matching
- `DraggableRuleList.svelte` - Drag-and-drop rule list with:
  - HTML5 native drag events
  - Visual drop indicator
  - Edit/delete actions on hover
  - Rule summary display
- Rewritten `/config/routes` page with:
  - Route settings section (final, auto_detect_interface)
  - Rule sets section with add/delete
  - Route rules section with drag reorder
  - Apply Changes button with loading state
- Enhanced `RouteRule` type with all match conditions
- Added `RouteSettings` type
- API client methods for all routing operations

**Documentation:**
- Added `docs/API.md` with complete API reference

---

### Phase 2: Outbounds & Inbounds (2026-01-22)

#### Added

**Backend:**
- Outbounds CRUD API (`/api/outbounds`)
- Inbounds CRUD API (`/api/inbounds`)
- Validation for outbound types (direct, block, selector, urltest, endpoint)
- Validation for inbound types (tun, mixed, socks, http)
- Reference validation (selector outbounds, endpoint_tag)

**Frontend:**
- `OutboundForm.svelte` - Form for all outbound types
- `InboundForm.svelte` - Form for TUN and proxy inbounds
- `/config/outbounds` page with full CRUD
- `/config/inbounds` page with full CRUD

---

### Phase 1: Core Infrastructure (2026-01-21)

#### Added

**Backend:**
- Go backend with chi router
- Config manager with load/save/validate
- Process manager for amnezia-box lifecycle
- Clash API proxy (HTTP + WebSocket)
- Endpoints CRUD API (`/api/endpoints`)
- AWG endpoint validation

**Frontend:**
- SvelteKit SPA with TypeScript
- Catppuccin theme (Mocha/Latte)
- Layout components (Sidebar, Header, ThemeToggle)
- Shared components (Modal, Toast notifications)
- API client with error handling
- `EndpointForm.svelte` for AWG configuration
- `/config/endpoints` page
- `/monitor/traffic` - Real-time traffic chart
- `/monitor/logs` - Live log viewer
- `/monitor/connections` - Active connections table

**Infrastructure:**
- Makefile for build automation
- go:embed for SPA bundling
- Single binary deployment
