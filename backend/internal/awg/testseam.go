package awg

// This file is a deliberate, narrow test seam (regular build, NOT _test.go) so
// the api package's external httptest suite can construct a Manager with a fake
// Runner and a seeded server key without reaching into unexported fields. It adds
// no production behaviour: callers in production use NewManager.

// NewManagerForTest builds a Manager backed by run, with secrets/.conf under
// baseDir, seeding the canonical render values (serverPriv/listenPort/mtu/dns) so
// RenderClientConf produces a complete client .conf. Peers are seeded via
// Store().Put. Intended for cross-package tests only.
func NewManagerForTest(run Runner, baseDir, serverPriv string, cfg Config) *Manager {
	m := NewManager(run, baseDir, cfg)
	m.serverPriv = serverPriv
	m.mtu = cfg.MTU
	m.dns = cfg.DNS
	m.listenPort = cfg.ListenPort
	return m
}
