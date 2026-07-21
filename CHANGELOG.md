# Changelog

All notable changes to RouteBox are documented here.

## [Unreleased]

### Features

- **Mieru inbound (server)** (#12) — RouteBox VPS panels can now run a [mieru](https://github.com/enfein/mieru)
  server. Add a **Mieru** inbound on `/config/inbounds` (listen port, TCP/UDP transport, one or more
  users with name+password, optional traffic pattern, and a "require user hint" toggle) — mieru carries
  its own encryption, so there is no TLS to configure. Users reconcile into the panel like every other
  server protocol: they appear on the Users page, in subscriptions, and in the share modal as `mierus://`
  links a client RouteBox imports (the outbound side shipped in 0.25.0), and they participate in per-user
  expiry/disable and traffic stats. Configs are validated before apply (transport, unique user
  names/passwords, no stray TLS/multiplexing); deleting the last user of a live mieru inbound is blocked
  (it would wedge every later apply) — delete the inbound instead. Requires amnezia-box
  `1.14.0-alpha.48-awg3-xhttp-mieru` or newer.

## [0.25.0] - 2026-07-21

### Features

- **Mieru outbound (client)** (#12) — RouteBox can now dial through a [mieru](https://github.com/enfein/mieru)
  proxy server. Add it manually on `/config/outbounds` (new **Mieru** type: server, TCP/UDP transport,
  port or port ranges, username/password, optional multiplexing level and traffic pattern) or import a
  `mierus://` link — a link carrying both TCP and UDP port groups asks which transport to use and imports
  a single outbound. Configs are validated against the fork's rules before apply (dash port ranges,
  multiplexing enum, base64 traffic pattern with a 64 KB cap), and applying a mieru config against an
  older amnezia-box binary without mieru support now fails with a clear "update the binary" hint instead
  of a cryptic decode error. Requires amnezia-box `1.14.0-alpha.48-awg3-xhttp-mieru` or newer.

### Fixes

- **Switching the AWG backend now actually decommissions the kernel runtime.** The
  kernel→singbox switch used to only check "server disabled" and flip the setting — an
  enabled-but-inactive `awg-quick@` unit, a manually-launched interface (`awg-quick up`
  outside the unit), or a broken `awg` tool all passed that check while the kernel tunnel
  stayed (or came back at next boot) alive, invisible to the panel and competing with the
  singbox endpoint for the UDP port. The switch now tears the old backend down first:
  `systemctl disable --now awg-quick@<iface>` (clears boot persistence), then `awg-quick
  down` for manually-launched interfaces, then `ip link delete` + explicit RBOX-AWG-*
  chain cleanup as a last resort — success is judged by the interface actually being gone,
  not by any one command's exit code. The reverse (singbox→kernel) switch likewise drops an
  orphaned managed endpoint from the active config.
- **Boot-time backend residue reconcile.** Installs already in the split state (settings say
  `singbox`, `awg-quick@` still enabled/running — or settings say `kernel` with a leftover
  managed endpoint in the config) are healed at RouteBox startup. Only interfaces RouteBox
  ever rendered a conf for are touched, so an operator's unrelated awg-quick setup is safe.
- **Kernel AWG Disable handles every launch variant** — non-systemd boxes and manually
  launched interfaces are now taken down too (previously only the systemd unit was stopped,
  and Disable reported success while the interface kept running).
- **Backend switch resets a stale `awg.enabled=true`** so a later RouteBox restart cannot
  rehydrate the newly selected backend as silently enabled.

## [0.24.1] - 2026-07-20

### Fixes

- **AWG-server client rows no longer overflow on mobile** (#16) — on narrow screens the per-client
  action buttons (Set expiry / QR / .conf / Export JSON / Download JSON / delete) were clipped off
  the right edge and the client address was squeezed to an ellipsis. The button group now wraps
  onto its own full-width row below the client name at ≤720px.

## [0.24.0] - 2026-07-20

### Features

- **QR and `.conf` export for sing-box AWG peers** — the AmneziaWG client `.conf` now carries the
  full awg3 field set (`HeaderProtectionKey`, `ContentPaddingAddition`, `RekeyAfterTime`), so it is
  a valid config against a sing-box AWG server. Sing-box peers on `/config/awg` now offer **QR** and
  **`.conf`** (for native AmneziaWG clients / phones) alongside the existing sing-box JSON export.
  awg3 obfuscation stays sing-box-only: the kernel enable path clears these fields so a classic
  `awg-quick` never sees an unknown key.

### Fixes

- **Clash API secret is honoured** — with `experimental.clash_api.secret` set, the panel now
  forwards it (`Authorization: Bearer …`) on every in-process Clash API call: the HTTP/WS proxy,
  the traffic-history sampler, LAN client discovery, and the sing-box version probe. Previously the
  secret was never sent, so sing-box replied 401 and the panel bounced you to the login screen when
  opening the Outbounds page, while traffic history silently recorded nothing and version detection
  degraded. A Clash upstream 401 is also no longer mistaken for a panel-session expiry, and the
  sampler now surfaces a non-200 as an error instead of storing zero connections.
- **Self-update restarts into the new binary** — after an in-app update RouteBox re-execed the
  *old* binary (it resolved the running executable via `os.Executable()`, which points at the
  `.old` backup once the swap renames files), so the UI kept showing the previous version. It now
  re-execs the freshly installed binary path.
- **AWG client configs work in router mode** — the `.conf` and sing-box JSON export on
  `/config/awg` no longer fail with *"public host not configured"* when no public domain is set
  (the usual case on a router, where clients connect to the LAN/WAN IP). A new **Server address**
  field on the AWG page sets the endpoint host written into exported client configs; it falls
  back to the panel's public host when left empty.
- **AWG status shows Running for a sing-box server** — an enabled sing-box AWG server no longer
  displayed *Stopped*. The status gate treated the kernel-only `iface_up` flag as required; it is
  now satisfied by the sing-box backend directly.
- **AWG status sub-line no longer shows a kernel interface name on sing-box** — the Running/Stopped
  sub-line showed `awg-rb0` (the kernel interface), which is meaningless for the sing-box backend;
  it now reads `sing-box` there.

## [0.23.1] - 2026-07-20

### Fixes

- **sing-box is the default AWG backend; router mode is sing-box-only** — a fresh install no
  longer defaults the AWG server to the **kernel** backend. This removes the kernel-module
  install (`apt-get install linux-headers-…`) from the default path — it was failing on
  non-standard kernels (e.g. Proxmox VE, where the package is `pve-headers-*`, not
  `linux-headers-*`). The `/config/awg` page stays available in **both** router and VPS mode,
  but in router mode the backend picker offers **sing-box only** — the kernel backend is
  selectable only in VPS mode.

  **Upgrade note:** if you ran the **kernel** AWG server on a **router** with the backend picker
  left untouched (so `awg.backend` is unset in `routebox.toml`), set `awg.backend = "kernel"`
  before upgrading — otherwise the panel resolves to sing-box on boot and the live `awg-rb0`
  interface is left unmanaged (disable it via `POST /api/awg/disable` or systemd).

## [0.23.0] - 2026-07-20

### Features

- **AWG server via sing-box (no kernel module)** — a second AmneziaWG *server* backend that runs
  through the amnezia-box fork's `type:"awg"` endpoint, selectable on `/config/awg` (kernel |
  sing-box). No kernel module required. The managed `awg-server` endpoint is change-gated into the
  active config; the peer roster (key generation, IPAM, expiry) is shared with the kernel path.
  Client configs are delivered as a sing-box endpoint JSON export (Copy / Download in the roster)
  and imported by pasting the JSON into the endpoint form. Available in both router and VPS mode.
- **AWG3 obfuscation** — `header_protection_key` (a 32-byte shared secret behind a Header
  Protection toggle; requires S1–S4 ≥ 8), `content_padding_addition`, and `rekey_after_time`.
  All three are emitted symmetrically in the server endpoint and every client export, and are
  feature-gated on an awg3-capable amnezia-box binary (older binaries never receive fields they
  would reject).
- **AWG server as a route-rule source** (#10) — the `awg-server` endpoint is now selectable as a
  *source* in route rules, so traffic arriving on the AWG server can be matched and routed (e.g.
  into the tun/VPN) from the web UI.
- **Drag & drop in Rule Set Based Routing** (#9) — assigned rule-set → outbound mappings can be
  reordered by priority (route-rule order, first-match-wins).
- **tun `auto_redirect` checkbox** (#11) — exposed in the tun inbound form (auto-configures the
  nftables redirect; the field the backend already accepted).

### Fixes

- **AmneziaWG kernel-module install on clean Ubuntu 24.04** (#14) — install `dirmngr` before the
  PPA signing-key fetch, so `gpg --recv-keys` no longer fails on a minimal image (no manual
  `apt install dirmngr` needed).
- **sing-box AWG server Disable now sticks** — a disabled server is no longer resurrected by the
  30-second expiry sweep or a RouteBox restart; the enabled state is persisted.
- **Backend switch blocked mid-enable** — switching kernel↔sing-box is rejected while an enable
  orchestrator is in flight, avoiding a live interface the panel can't represent.
- **Deterministic sweep-loop stop** — `RunSweepLoop` prioritizes the stop signal (removes a
  race-detector flake).

### Deploy notes (VPS)

- A sing-box AWG server enabled on a build **before this release** must be re-enabled once after
  updating (or set `enabled = true` under `[awg]` in `routebox.toml` before restart) — otherwise
  it rehydrates as disabled and the sweep removes the endpoint.
- Header Protection derives a shared secret; toggling it or rotating the key invalidates
  previously exported client configs — re-export them.

## [0.22.0] - 2026-07-14

### Features

- **DNS detour via AWG/WG endpoint** (#7) — the DNS server "detour" picker now lists AmneziaWG /
  WireGuard endpoints alongside outbounds (endpoints act as outbounds in sing-box, same as in
  route rules), and the backend accepts endpoint tags as a valid `detour` reference.
- **Endpoints section available in VPS mode** (#7) — `/config/endpoints` is no longer router-only,
  so a VPS panel can chain its traffic through an upstream AWG/WG endpoint without editing
  `routebox.toml` to temporarily switch modes.

### Fixes

- **Breakdown range switcher no longer clips "Month"** (#3) — the Live/1h/3h/24h/Week/Month pills
  now size to their labels instead of six equal truncating columns.
- **Breakdown domain drill-down got a back button** (#3) — a slim "Back to all domains" /
  "Back to {domain}" row at the top of the By Domain panel steps back one level, instead of
  relying on the filter chip or re-clicking the active row.
- **Chain badges are no longer bold in "Group by chain"** (#3) — the chip style now pins its own
  font weight, so badges look the same everywhere.
- **Dashboard "Managed by systemd (unit)" spacing** (#3) — the unit name no longer glues to the
  word "systemd".
- **NaiveProxy "Import from link" is a proper button** (#3) — it now matches the dashed import
  button used by the VLESS / Trojan / Hysteria2 / Shadowsocks forms instead of a bare text link.

## [0.21.1] - 2026-06-26

### Fixes

- **Self-update no longer kills the panel on `Restart=on-failure` units** — the in-app updater
  used to swap the binary and `os.Exit(0)`, assuming systemd would respawn it. Units installed
  with `Restart=on-failure` do not respawn on a clean exit, so the panel went down and stayed down
  after an update ("crash"). It now re-execs the running process onto the new binary in place
  (`syscall.Exec`, same PID): restart works under any `Restart=` policy and even outside systemd.
  Existing installs still need this binary before the fix takes effect — update once more, then
  future self-updates restart cleanly. New install/VPS units now ship `Restart=always`.

## [0.21.0] - 2026-06-26

### Features

- **Single-connection share is back** — the per-user **Share** action now opens a modal with a
  Subscription / Single connection toggle. "Single connection" lets you pick one protocol binding
  and get its direct `vless://` / `trojan://` / `naive://` / `hysteria2://` link + QR, for clients
  that don't take a subscription or when you want to add just one protocol to a device. The
  subscription mode (token URL, QR, rotate, revoke) is unchanged.
- **AmneziaWG config expiry + manual renewal** — each AWG peer can be given an expiry date. When it
  lapses the peer is **suspended** (removed from the live interface, but its keys and IP are kept),
  so renewing re-admits the same config and the client's existing `.conf` reconnects instantly. A
  background sweep enforces expiry (~30s); the roster shows the expiry date, a **Suspended** badge,
  and an inline **Renew** control (date picker + +30d / +90d / Never). New peers are created
  never-expiring. Endpoint: `PATCH /api/awg/peers/{publicKey}/expiry`.

### Changes

- **Dashboard inbounds hint** now reads `VLESS / NaiveProxy / Hysteria2 / Trojan` (Trojan added, the
  trailing "listeners" word dropped).
- **Inbound TLS mode button** renamed from "Panel cert" to **"Panel Certificate"**.
- **AWG obfuscation fields** are grouped onto their own rows (J / S / H) instead of one flat grid,
  so all S and all H parameters line up per row.

## [0.20.3] - 2026-06-18

### Bug Fixes

- **AmneziaWG client configs no longer carry a phantom `Itime` field** — the CPS obfuscation
  builder emitted `Itime = N`, which is not part of the AmneziaWG 2.0 spec (valid CPS fields are
  `I1`-`I5`). The bogus line is dropped from generated client configs; only `I1`-`I5` are written.
- **Self-update now restarts the panel** — the systemd unit shipped `Restart=on-failure`, so the
  clean `exit(0)` after a binary swap left the panel down instead of respawning. The installers now
  set `Restart=always` (`RestartSec=2`). Existing installs must re-run the installer (or set
  `Restart=always` + `systemctl daemon-reload`) to pick this up.

## [0.20.2] - 2026-06-17

### Changes

- **Subscription is the single share action for a user** — the per-binding "Client link"
  button is removed; the panel now exposes one primary **Subscription** action that returns the
  user's whole node set (all protocols) as a standard base64 subscription, parsed by the client.
  This drops the confusing per-binding link (which only ever emitted the first binding).

### Bug Fixes

- **AmneziaWG status reflects the live interface**, not the boot-time snapshot — the status now
  trusts the running `awg-rb0` state instead of stale module state captured at startup.

## [0.20.1] - 2026-06-17

### New Features

- **Reuse the panel's TLS certificate for inbounds ("Panel cert" mode)** — a new TLS mode for vless/trojan/naive/hysteria2 inbounds that reuses the panel's Let's Encrypt certificate instead of running a second ACME or pasting a manual cert. RouteBox mirrors the panel cert to a canonical path (`/etc/routebox/panel-cert/{fullchain.pem,key.pem}`), keeps it current, and SIGHUPs amnezia-box on renewal so inbounds pick up the new cert in place. Available whenever the panel itself has a cert (ACME or manual). All inbound variants (vless reality/ws/grpc/httpupgrade, trojan, naive, hysteria2) were live-verified end-to-end against this cert.
- **Attach an existing panel user when adding inbound users** — the inbound editor can bind an already-existing user (by name) instead of forcing a brand-new one, so a person's bindings across inbounds aggregate into one subscription token rather than fragmenting into separate subs.

### Bug Fixes

- **amnezia-box no longer crash-loops on a fresh VPS install** — in vps mode RouteBox scaffolds a minimal valid sing-box config on boot when `/etc/amnezia-box/config.json` is absent, so the enabled service starts clean instead of crash-looping until the first apply. The scaffold ships a `direct` outbound plus a `route` block (`auto_detect_interface: true`, `final: direct`) so egress works out of the box.
- **Reject duplicate `(listen, port)` across inbounds** — config validation now blocks two inbounds sharing the same listen address and port (e.g. naive and vless both on 443), which previously left the second one silently broken.
- **Inbound form validation names and highlights the missing field** — instead of a mute "This field is required" toast, the message now names the field (e.g. "Handshake Server (SNI) is required", or "Generate a Reality keypair first") and the offending input is highlighted, across all TLS modes, the port, and the users block.
- **Panel-cert export hardened** — the cert is read directly from autocert's on-disk cache (deterministic) rather than via a synthetic `GetCertificate` call that didn't export on the live stand; a 5-minute poll catches first issuance and renewals.
- **"Panel cert" inbound is saveable** — the form's TLS validation no longer falls through to the manual branch and demands a Certificate Path that panel mode injects automatically, which had blocked saving any panel-cert inbound from the UI.
- **Duplicate listen port is rejected on add/edit, not just full-config validate** — `CreateInbound`/`UpdateInbound` now enforce a unique `(listen, port)` (the cross-inbound check previously ran only in the whole-config validator, so the per-inbound API path let you stage e.g. a second inbound on 443 and crash amnezia-box on reload). Wildcard listen spellings are normalized, so the panel's `::` correctly collides with a legacy inbound that omits `listen`.

## [0.19.0] - 2026-06-16

### New Features

- **Configurable AmneziaWG server page (`/config/awg`)** — replaced the one-button MVP with a state-aware page: a guided setup wizard (Setup → Configure → Turn on → Share) until the first enable, then a steady "manage clients" view. Editable server settings (listen port, subnet, MTU, DNS, WAN interface) with Save, Enable/Disable, and Apply-on-change (re-renders + restarts the interface when saved settings differ from the running one).
- **AmneziaWG obfuscation — now actually applied, with profiles.** Obfuscation values were previously never threaded through, so the server ran as plain WireGuard; they now flow into both the server `.conf` and every client `.conf` (always matching). Four profiles — **Off / DNS / Web / Stealth** — materialise `Jc/Jmin/Jmax/S1–S4` from researched ranges (randomised per server), with `H1–H4` emitted as per-quadrant ranges (`lo-hi`) so the header is randomised per packet instead of being a static signature.
- **CPS protocol mimicry (`I1–I5` + `Itime`)** — generated client configs carry real protocol-mimicry packets matching the profile (DNS query / Chrome-like TLS ClientHello with GREASE + random SNI / SIP REGISTER), built in pure Go (`awg/cps`). These are client-only; the server `.conf` stays I-free (so our `awg-quick`/kernel module never parses them).
- **Live per-client + server status** — real "Connected" count and per-peer online badges derived from live handshakes (`awg show latest-handshakes`), cumulative per-peer + server traffic totals (`awg show transfer`), and a larger, lower-density QR that stays scannable for the bigger obfuscated configs.
- **VPS sidebar cleanup** — AmneziaWG moved to the top of Config, Users moved before Experimental, and the client-only Endpoints page is hidden in panel mode.

### Bug Fixes

- **Apply/re-enable now actually applies** — `iface_Up` used `systemctl enable --now`, a no-op on an already-active unit, so port/obfuscation/subnet changes silently never reached the live interface; it now `enable`s (boot) + `restart`s (reload).
- AWG enable fixes: `listen_port` JSON decoding (was silently 0), conf path `/etc/amnezia/amneziawg` (where `awg-quick@` reads), and the NAT health-gate/status check the `nat` table.
- QR/`.conf` endpoint URL-decodes the peer public key (was HTTP 400 on any key containing `+`, `/`, or `=`).
- Manager rehydrates render state on boot (client configs no longer 500 after a restart until a re-enable); existing peers are preserved in the rendered `.conf` across Apply/reboot.
- Enable reads the persisted settings instead of the request body; `obf_preset` participates in change-detection; `validateObf` enforces `Jmin < Jmax` and `S1 + 56 ≠ S2`.

## [0.18.0] - 2026-06-15

### New Features

- **Trojan inbound/outbound + transport selector** — Trojan is now a first-class server inbound and client outbound, and vless/trojan gain a pluggable stream transport on both sides. See details below.

**Backend:**
- `serverlinks`: new `buildTrojan` (Trojan client share-link) and transport params (`type`/`path`/`host`/`serviceName`) emitted into vless and trojan links.
- `users.CredentialKey`: trojan inbound credential is the peer `password`.
- Config validator: validates trojan inbounds (TLS-or-Reality required, duplicate-password rejected, inbound `utls` rejected) and trojan outbounds; enforces the **Vision↔transport** constraint — `flow: xtls-rprx-vision` requires raw TCP, so a non-raw transport strips/forbids the flow (`amnezia-box check` does NOT catch this; the RouteBox validator does).
- `subscriptions.ParseLinks` now parses `trojan://` links and the `httpupgrade` transport (RouteBox-as-client subscription import).
- Transport is orthogonal to TLS — `vless+reality+grpc`, `trojan+reality+ws`, etc. are all valid. Host handling is per transport: `ws`→`headers.Host`, `httpupgrade`→top-level `host`, `grpc`→`service_name`.

**Frontend:**
- Trojan server type in InboundForm (Reality/ACME/Manual TLS, password credential) and a new TrojanForm client outbound (Reality/standard TLS) with `trojan://` import in the outbound Import modal.
- Stream-transport selector (`raw` / `ws` / `grpc` / `httpupgrade`) for vless & trojan on both the server-inbound and client-outbound sides; `httpupgrade` also added to the existing vless outbound transport options.

Inbounds/outbounds flow through the existing config CRUD + `POST /api/config/apply` — no new REST endpoints.

---

- **Panel security: change password + username** — operators can now change the panel password and username from the UI. See details below.

**Backend:**
- New protected `POST /api/auth/change-password` (`{current_password, new_password}`): re-verifies the current password (bcrypt) under the same brute-force lockout as login, requires `new_password` length ≥ 8, then bcrypt-hashes and persists the new password (Update + Save). The current session stays valid (no forced re-login).
- Username change reuses `PUT /api/settings` (`security.auth_username`) — no new endpoint.

**Frontend:**
- New **Security** section on `/config/app` to change the password (current/new/confirm) and username.

---

- **AmneziaWG server inbound (VPS panel)** — RouteBox can now run an AmneziaWG VPN server itself, independent of sing-box. It installs the `amneziawg` kernel module on demand, brings up its own `awg-rb0` interface, and provisions client peers live. See details below.

**Backend:**
- New `awg` package: a `Runner`-backed manager that owns the `awg-rb0` interface, the peer secret store, and all privileged operations. Enable orchestrator runs validate → install module → keygen → render → `awg-quick up` → health-gate, with full teardown on any failure (no orphan NAT, no orphan interface).
- On-demand kernel-module installer for Debian-family hosts: adds the AmneziaWG PPA with a **pinned** signing-key fingerprint (resists repo/key-server compromise), single-flight so concurrent enables can't double-install.
- Interface managed via `awg-quick` with NAT confined to dedicated `RBOX-AWG-*` iptables chains (created in PostUp, fully reverted in PostDown), so RouteBox's masquerade rules never collide with the host's.
- Live peer provisioning via `awg set` (no tunnel flap / no existing-client disruption): each peer gets a `/32` from an order-independent lowest-free IPAM allocator, a generated WG keypair, and a re-showable client `.conf` (QR-able). Peer secrets persisted `0600` with bidirectional reconcile between the on-disk store and the live interface.
- **Security-critical input-validation layer:** every operator-controlled field (`subnet`, `wan_iface`, `listen_port`, `name`, `dns`, `mtu`, `allowed_ips`, the AmneziaWG H1–H4 / J / S obfuscation fields, peer `publicKey`) is validated and canonically re-emitted before it can reach the root shell — `.conf`/`awg-quick`/`iptables` see only canonical values, never raw input.
- `/api/awg/*` endpoints: `GET status`, `POST enable`/`disable`, `GET`/`POST peers`, `DELETE peers/{publicKey}`, and `GET peers/{publicKey}/config` (serves the client `.conf` as `text/plain`, hardened like `/sub`). Interface settings persisted in RouteBox settings; peers in `peers.toml` under `/etc/routebox/amneziawg`.

**Frontend:**
- New `/config/awg` panel page: module/interface status, an enable form (subnet, listen port, MTU, DNS), a peer roster with live add/remove, and per-peer config download + QR.

**VPS/panel mode only and fully additive** — router-mode installs are unchanged, and a binary/host without the feature simply doesn't expose it.

---

- **Per-user traffic accounting (VPS Phase 5)** — RouteBox now records accurate per-user upload/download via sing-box's v2ray_api StatsService. See details below.

**Backend:**
- New `v2stats` gRPC client polling the StatsService; `user_traffic` table + reset-safe cumulative-diff sampler (survives sing-box reloads without losing/negativing totals).
- RouteBox writes & syncs the `experimental.v2ray_api` block (`stats.users` kept in sync with the panel registry, change-gated to avoid reload churn; loopback-only listener).
- `GET /api/users/{id}/traffic` (totals + per-bucket history) and up/down sums in `GET /api/users`; `Subscription-Userinfo` header on `/sub`.

**Frontend:**
- Per-user up/download counters + sparkline on the Users page; a per-user traffic monitor (the panel-mode "Part B" view).

**Requires** an amnezia-box build with `with_v2ray_api` (fork release `v1.13.13-awg2.1`); additive — on a binary without it, accounting is simply absent. Quotas/auto-disable are out of scope (future, needs Phase 4 enforcement).

---

- **Mode-aware UI (panel vs router)** — the WebUI now adapts to `server.mode` (router|vps). See details below.

**Frontend:**
- In **vps/panel** mode the sidebar hides router/LAN-only sections (Clients, upstream Subscriptions, and Monitor → Traffic/Breakdown/Proxies/Route-inspector) and shows Users; the inbound type picker offers only server types (vless/naive/hysteria2); the Overview shows a panel summary (user count, public host/TLS); App Settings exposes panel-only TLS/ACME fields.
- In **router** mode the full existing UI is shown (minus Users). A mode toggle in App Settings switches live (no reload). A soft redirect sends you to the Overview if you open a route not available in the current mode.
- Additive: default `router` → existing installs are unchanged. Per-user panel monitoring is deferred (Part B, after per-user traffic accounting).

---

- **VPS deploy (embedded ACME)** — deploy RouteBox as a public TLS admin panel on a VPS with one script. RouteBox issues and auto-renews its own Let's Encrypt certificate in-process (HTTP-01 on :80) — no certbot, no nginx — coexisting with vless+Reality on :443, and the subscription URL carries the panel port so clients reach the panel rather than :443. Verified end-to-end on a live VPS (LE staging + production cert issued by RouteBox, panel on :8443 coexisting with vless on :443). See details below.

**Backend:**
- Embedded ACME TLS via `golang.org/x/crypto/acme/autocert`: RouteBox issues and hot-renews Let's Encrypt certs itself (HTTP-01 challenge on :80), no external certbot.
- New TLS modes off/manual/acme driven by `network.acme_enabled`/`acme_email`/`acme_staging`/`acme_cache_dir` (default cache `/etc/routebox/acme`); `resolveTLSMode` resolves with priority acme→manual→off, fatal if acme is enabled without `server.public_host`.
- `HostPolicy` whitelist restricts issuance to the configured domain (anti-abuse); staging directory selected when `acme_staging` so dev/e2e never touches prod issuance.
- New `Server.PublicPort` so the subscription URL carries the panel's TLS port when the panel isn't on 443.

**Frontend:**
- Subscription URL includes the panel public port (`public_port`) when set and ≠443 (IPv6-safe), so `/sub` links work when the panel coexists with vless on 443.

**Deployment (release-repo):**
- New `vps-install.sh` — one-shot VPS install (binaries + sha256, GeoIP, acme-mode config, systemd units, firewall) with hybrid flags/interactive input, DNS precheck, and `--staging`/`--update`/`--uninstall`; clears the ACME cache on staging↔prod switch.

---

- **Server inbounds (VPS mode)** — vless/naive/hysteria2 server-side inbound support with full TLS configuration (ACME, Reality, or manual certificate), per-user credential management, and QR/share-link generation. See details below.

**Backend:**
- `serverlinks` package: builds vless/naive/hysteria2 client share-links from server inbounds; Reality public key is derived from the stored private key via X25519 (no extra config field required).
- Credential generators: `POST /api/generate/reality` (via detected binary), `/api/generate/uuid`, `/api/generate/password`.
- `GET /api/inbounds/{tag}/users/{userKey}/link` — per-user share link endpoint.

**Frontend:**
- InboundForm now supports vless/naive/hysteria2 server types with ACME/Reality/Manual TLS, user management with credential generation, and a QR share-link modal (`qrcode`).

---

- **Subscriptions** — manage proxy subscriptions from the UI: add a subscription by name + URL, RouteBox fetches the base64 link list (`ss://`, `vless://`, …), parses each into an outbound, and merges them into the working config as a `urltest` group named after the subscription with `«Name» · «Node»` member tags. On-demand refresh and an hourly auto-refresh ticker (`interval_hrs`) atomically replace that subscription's nodes — a transient empty/failed fetch leaves the existing group untouched. Deleting a subscription removes its group and nodes from the draft. New `CRUD /api/subscriptions` + `POST /api/subscriptions/{id}/refresh` endpoints and a TOML-backed store (`subscriptions.toml`).
- **Updates page** — check, download and install new releases of amnezia-box and RouteBox from the UI: GitHub release checking (daily auto-check, `updates.auto_check`), sha256+ELF+smoke verification, atomic binary swap with automatic rollback, systemd-supervised RouteBox self-update. Release assets now ship sha256 checksums.

---

### Added — Subscription export (VPS Phase 3)

Per-user subscription URLs: each panel user gets an opaque token and a public `GET /sub/{token}` endpoint that returns the user's client share-links (base64), so clients can self-provision and auto-update from a single link.

**Backend:**
- Per-user subscription token auto-minted during reconcile (`base64url` of 32 `crypto/rand` bytes), idempotent (only mints when empty) and stored in `PanelUser.Token`. A1 (user removed from active config) deletes the record and its token; re-adding the user mints a fresh token.
- `users.ByToken` (deep-copy value lookup, empty token rejected), `RotateToken`, and `RevokeToken`. Revoke is **sticky / fail-closed**: it clears the token and sets `TokenDisabled=true`, so a subsequent reconcile does **not** re-mint a token for that user (verified end to end across a restart).
- Host-agnostic `BuildSubscription`: base64 of the user's client share-links built from the **active** config and `Server.PublicHost`; bindings whose inbound is missing from the active config are gracefully skipped. Self-contained (no API resolvers).
- **PUBLIC** `GET /sub/{token}` (registered outside auth): identical `404` for unknown and revoked tokens (anti-enumeration), `503` when `server.public_host` is unset, dedicated per-IP rate-limit (Allowed+Fail on every request), `Cache-Control: no-store`, `Content-Type: text/plain`, `Profile-Update-Interval`, and `Content-Disposition` headers. Access-log token scrubbing renders the path as `/sub/<redacted>` so the token never reaches logs.
- **PROTECTED** `POST /api/users/{id}/token/rotate` and `DELETE /api/users/{id}/token`; `token` (and `token_disabled`) added to `GET /api/users`.

**Frontend:**
- Subscription modal on the Users page: subscription URL + QR code + copy, plus rotate and revoke actions. Scheme-less public hosts are forced to `https`. Hints shown when the public host is unset or the user has no token yet (pending users).

---

### Added — Panel users (VPS Phase 2)

A unified Users registry that mirrors the active server config. Panel users are kept in a `users.toml` sidecar (next to `routebox.toml`, mode 0600) that is reconciled against the live config on startup and after every Apply, so the panel always reflects what amnezia-box is actually running.

**Backend:**
- `users` package: TOML-backed registry (`PanelUser`/`Binding`) with atomic 0600 saves, deep-copy reads, and a pure server-inbound credential extractor (vless=uuid, naive=username, hysteria2=password via a shared `CredentialKey` helper).
- Reconciler mirrors the **active** config into the registry: imports newly-seen credentials with a fresh stable id, updates cached metadata, collapses duplicate credentials, preserves multi-binding users, and **auto-deletes** registry entries whose credential vanished from the active config (A1). Idempotent; runs at startup and after Apply, outside the config manager's lock (no import cycle, `users` → `config` only).
- `CRUD /api/users`: `GET` returns ONE unified list (registered users plus draft-only entries flagged `pending:true` with an empty id); `POST` stages a new user (generated credential) into the draft inbound only; `POST /api/users/{id}/bindings` adds the user to another inbound's draft; `DELETE /api/users/{id}` removes the draft bindings (registry cleaned by reconcile on Apply).
- `GET /api/users/{id}/link?tag=&host=` builds a client share link by user id, resolved against the **active** config; `host` defaults to `server.public_host` when omitted and is passed through the same sanitizer.
- New `server.public_host` setting with a strict sanitizer (strips scheme/userinfo/path/port/trailing dot, rejects non-hostname/non-IP input).
- Validator hardening for server inbounds: `listen_port` required, naive/hysteria2 require TLS **enabled**, and duplicate credentials within an inbound are rejected.
- Removed the old index-based per-inbound share-link route and handler.

**Frontend:**
- New **Users** page (`/config/users`) and nav entry: unified list with a pending badge, Add user, Delete, and a Client-link action.
- Share-link modal repurposed to the by-id link route with `server.public_host` prefill (typed; `PanelUser`/`PanelBinding`/`ServerSettings` added to `types.ts`, no `as any`).
- API client gains `getUsers`/`createUser`/`addUserBinding`/`deleteUser`/`getUserLink(id, tag, host)`.
- Public host field added to the Settings page; `users.*` i18n namespace added.
- Removed the per-inbound share button and its wiring from the server inbound forms.

### Added — Panel security (VPS Phase 1)

**Backend:**
- Cookie-based login sessions (`POST /api/auth/login` / `logout`, `GET /api/auth/session`) with sliding expiry; HTTP Basic still accepted for scripts.
- Passwords hashed with bcrypt at rest (existing plaintext `auth_password` auto-migrated on load) + brute-force lockout with exponential backoff.
- Root router split into public (login/health/SPA) and authenticated groups.
- Optional built-in TLS via `network.tls_cert_path` / `tls_key_path`.
- Hardened defaults: same-origin CORS (default no wildcard), same-origin WebSocket origin checks, panel credentials stripped before proxying to the clash API, `no-store` on auth responses.

**Frontend:**
- Login page, automatic redirect to login on 401, logout control, and an insecure-HTTP warning banner.

### Changed

- **BREAKING (VPS mode only):** `--mode=vps` (or `server.mode = "vps"`) now force-enables auth and generates an admin password (printed to the log and written to `routebox-initial-password`, mode 0600) if none is configured. Default `router` mode is unchanged; a warning is logged when bound to a non-loopback address without auth.
- CORS now defaults to same-origin (no `Access-Control-Allow-Origin: *`); set `security.cors_origins` to opt into cross-origin access.

---

## [0.17.0] - 2026-06-12

### New Features

**Frontend:**
- **NaiveProxy outbound type** — full form with server/port, username/password, SNI, CA certificate (PEM) pinning, QUIC transport toggle, and extra HTTP headers. Supports import from `naive+https://` and `naive+quic://` URIs via the Import modal.
- **Mobile layout pass** — connection cards on phones, bottom-sheet modals, responsive grids on dashboard/traffic/connections, stacked pie-chart legend, wrapping proxy cards, stacked unsaved-changes bar

**Backend:**
- `naive` outbound tag recognized in the VPN-config detection heuristic.

> **Note:** Requires amnezia-box `v1.13.13-awg2.0` or newer linux-amd64 binary (naive outbound is amd64-only).

### Bug fixes

- **Backend:** process manager no longer matches RouteBox itself when finding the proxy PID; config validation uses the detected amnezia-box binary; atomic config writes (power-loss safe); numeric settings updates no longer silently ignored; process lifecycle serialized; optional HTTP basic auth (settings `[security]`) + server timeouts; WS proxy keepalive (no more leaked connections); backup rotation (keep 5)
- **Frontend:** monitor streams auto-reconnect with honest status indicators (works behind HTTPS); dashboard start/stop manages both streams; live-updates toggle survives transient errors; proxies auto-refresh respects interval changes; `ss://` base64url import; backup import correctly reports draft state; missing i18n keys; confirmation for Close All connections and endpoint delete
- **Backend:** process status polling no longer forks per call (version cache, Signal(0) liveness); draft apply guarded against concurrent edits (409); graceful shutdown flushes client data; Clash API calls bounded by timeouts; orphaned temp files swept at startup; note: `/api/health` requires credentials when basic auth is enabled
- **Frontend:** stream backoff no longer busy-loops against a stopped proxy; IPv6 hosts in import links; NaiveProxy naming unified; "Servers" section; restart confirmation; localized remaining notifications and empty states; honest pending-changes counter; safe-area padding on mobile sheets

---

## [0.16.2] - 2026-05-10

### Improvements

**Frontend:**
- **Breakdown — Min volume filter** — new dropdown in the page header (next to Reset history) lets you hide buckets below a chosen size threshold (presets 10 MB / 100 MB / 1 GB or a custom value with KB/MB/GB units). Filter is display-only — affects rows, pie chart segments, and panel counts in all three panels (BY SOURCE / BY DOMAIN / BY CHAIN), but does not change the page-level "Filtered total" cumulative numbers. The chosen threshold persists per-device in `localStorage`.
- **Breakdown — mobile layout polish** — the totals row no longer splits value+unit pairs across lines on narrow viewports, and the time-range pill group wraps onto a 2nd row instead of overflowing the right edge.

### Bug fixes

**Frontend:**
- **Breakdown — historical "Filtered total" now matches the panel sums when an apex domain is filtered.** Previously, picking a filter like `googlevideo.com` on the 1h/3h/24h/Week/Month views would show `0 B` at the top even though the BY SOURCE panel correctly aggregated tens of MB. The totals row was doing exact-match against the filter while the panels did apex-match (since the v0.16.1 apex rollup); aligning the totals row to use the same apex match resolves the discrepancy.
- **Logs page — respect the saved log level on initial mount.** The dropdown restored its value from `localStorage` but the stream briefly subscribed to `info` before the reactive effect re-opened it at the saved level. Now the stream starts at the right level immediately.

---

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
