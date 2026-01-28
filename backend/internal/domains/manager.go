package domains

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// RuleSetSource represents a JSON source file for sing-box rule-set compilation
type RuleSetSource struct {
	Version int            `json:"version"`
	Rules   []HeadlessRule `json:"rules"`
}

// HeadlessRule matches sing-box headless rule format
type HeadlessRule struct {
	Domain        []string `json:"domain,omitempty"`
	DomainSuffix  []string `json:"domain_suffix,omitempty"`
	DomainKeyword []string `json:"domain_keyword,omitempty"`
	DomainRegex   []string `json:"domain_regex,omitempty"`
	IPCidr        []string `json:"ip_cidr,omitempty"`
	SourceIPCidr  []string `json:"source_ip_cidr,omitempty"`
	Port          []int    `json:"port,omitempty"`
	PortRange     []string `json:"port_range,omitempty"`
	ProcessName   []string `json:"process_name,omitempty"`
	ProcessPath   []string `json:"process_path,omitempty"`
}

// SetInfo is returned by ListSets
type SetInfo struct {
	Tag            string `json:"tag"`
	DomainCount    int    `json:"domain_count"`
	RuleCount      int    `json:"rule_count"`
	HasCompiled    bool   `json:"has_compiled"`
	NeedsRecompile bool   `json:"needs_recompile"`
}

// Manager handles rule set source files on disk
type Manager struct {
	dir        string // ruleset/ directory path
	binaryName string // amnezia-box binary name
	mu         sync.RWMutex
}

// tag validation: alphanumeric, hyphens, underscores
var tagRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

// domain validation: RFC-compliant domain pattern
var domainRegex = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?\.)*[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?$`)

// NewManager creates a domains manager. configPath is the sing-box config file path,
// binaryName is the amnezia-box binary name.
func NewManager(configPath string, binaryName string) *Manager {
	dir := filepath.Join(filepath.Dir(configPath), "ruleset")
	return &Manager{
		dir:        dir,
		binaryName: binaryName,
	}
}

// ensureDir creates the ruleset directory if it doesn't exist
func (m *Manager) ensureDir() error {
	return os.MkdirAll(m.dir, 0755)
}

func (m *Manager) jsonPath(tag string) string {
	return filepath.Join(m.dir, tag+".json")
}

func (m *Manager) srsPath(tag string) string {
	return filepath.Join(m.dir, tag+".srs")
}

// ListSets returns metadata for all rule set source files
func (m *Manager) ListSets() ([]SetInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if err := m.ensureDir(); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(m.dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read ruleset dir: %w", err)
	}

	var sets []SetInfo
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		tag := strings.TrimSuffix(entry.Name(), ".json")
		src, err := m.readSetUnlocked(tag)
		if err != nil {
			continue
		}

		info := SetInfo{
			Tag:       tag,
			RuleCount: len(src.Rules),
		}

		// Count domain entries across all rules
		for _, rule := range src.Rules {
			info.DomainCount += len(rule.Domain) + len(rule.DomainSuffix)
		}

		// Check compiled file
		jsonInfo, _ := os.Stat(m.jsonPath(tag))
		srsInfo, srsErr := os.Stat(m.srsPath(tag))
		info.HasCompiled = srsErr == nil
		if info.HasCompiled && jsonInfo != nil {
			info.NeedsRecompile = jsonInfo.ModTime().After(srsInfo.ModTime())
		} else {
			info.NeedsRecompile = true
		}

		sets = append(sets, info)
	}

	if sets == nil {
		sets = []SetInfo{}
	}
	return sets, nil
}

// GetSet reads the full rule set source file
func (m *Manager) GetSet(tag string) (*RuleSetSource, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.readSetUnlocked(tag)
}

func (m *Manager) readSetUnlocked(tag string) (*RuleSetSource, error) {
	data, err := os.ReadFile(m.jsonPath(tag))
	if err != nil {
		return nil, fmt.Errorf("rule set '%s' not found", tag)
	}

	var src RuleSetSource
	if err := json.Unmarshal(data, &src); err != nil {
		return nil, fmt.Errorf("failed to parse rule set '%s': %w", tag, err)
	}
	return &src, nil
}

// CreateSet creates an empty rule set source file
func (m *Manager) CreateSet(tag string) error {
	if !tagRegex.MatchString(tag) {
		return fmt.Errorf("invalid tag: must be alphanumeric with hyphens/underscores, got '%s'", tag)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.ensureDir(); err != nil {
		return err
	}

	path := m.jsonPath(tag)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("rule set '%s' already exists", tag)
	}

	src := RuleSetSource{
		Version: 3,
		Rules:   []HeadlessRule{},
	}
	return m.writeSetUnlocked(tag, &src)
}

// DeleteSet removes both the JSON source and compiled SRS file
func (m *Manager) DeleteSet(tag string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	jsonPath := m.jsonPath(tag)
	if _, err := os.Stat(jsonPath); os.IsNotExist(err) {
		return fmt.Errorf("rule set '%s' not found", tag)
	}

	os.Remove(jsonPath)
	os.Remove(m.srsPath(tag))
	return nil
}

// SaveSet writes the full rule set source (for JSON mode saves)
func (m *Manager) SaveSet(tag string, src *RuleSetSource) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, err := os.Stat(m.jsonPath(tag)); os.IsNotExist(err) {
		return fmt.Errorf("rule set '%s' not found", tag)
	}

	// Ensure version is set
	if src.Version == 0 {
		src.Version = 3
	}

	return m.writeSetUnlocked(tag, src)
}

// AddDomain adds a domain to the first rule's domain_suffix field
func (m *Manager) AddDomain(tag string, domain string) error {
	domain, err := validateDomain(domain)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	src, err := m.readSetUnlocked(tag)
	if err != nil {
		return err
	}

	// Ensure at least one rule exists
	if len(src.Rules) == 0 {
		src.Rules = []HeadlessRule{{}}
	}

	// Check for duplicates across all rules
	for _, rule := range src.Rules {
		for _, d := range rule.DomainSuffix {
			if d == domain {
				return fmt.Errorf("domain '%s' already exists", domain)
			}
		}
	}

	// Add to first rule's domain_suffix
	src.Rules[0].DomainSuffix = append(src.Rules[0].DomainSuffix, domain)

	return m.writeSetUnlocked(tag, src)
}

// RemoveDomain removes a domain from domain_suffix across all rules
func (m *Manager) RemoveDomain(tag string, domain string) error {
	domain = strings.ToLower(strings.TrimSpace(domain))

	m.mu.Lock()
	defer m.mu.Unlock()

	src, err := m.readSetUnlocked(tag)
	if err != nil {
		return err
	}

	found := false
	for i := range src.Rules {
		filtered := make([]string, 0, len(src.Rules[i].DomainSuffix))
		for _, d := range src.Rules[i].DomainSuffix {
			if d == domain {
				found = true
			} else {
				filtered = append(filtered, d)
			}
		}
		src.Rules[i].DomainSuffix = filtered
	}

	if !found {
		return fmt.Errorf("domain '%s' not found", domain)
	}

	return m.writeSetUnlocked(tag, src)
}

// ImportDomains bulk-adds domains to domain_suffix in the first rule
func (m *Manager) ImportDomains(tag string, domains []string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	src, err := m.readSetUnlocked(tag)
	if err != nil {
		return 0, err
	}

	// Ensure at least one rule exists
	if len(src.Rules) == 0 {
		src.Rules = []HeadlessRule{{}}
	}

	// Build existing set for dedup
	existing := make(map[string]bool)
	for _, rule := range src.Rules {
		for _, d := range rule.DomainSuffix {
			existing[d] = true
		}
	}

	added := 0
	for _, raw := range domains {
		domain, err := validateDomain(raw)
		if err != nil {
			continue // skip invalid
		}
		if existing[domain] {
			continue // skip duplicates
		}
		existing[domain] = true
		src.Rules[0].DomainSuffix = append(src.Rules[0].DomainSuffix, domain)
		added++
	}

	if err := m.writeSetUnlocked(tag, src); err != nil {
		return 0, err
	}
	return added, nil
}

// Compile runs the sing-box rule-set compile command
func (m *Manager) Compile(tag string) error {
	m.mu.RLock()
	jsonPath := m.jsonPath(tag)
	srsPath := m.srsPath(tag)
	binaryName := m.binaryName
	m.mu.RUnlock()

	if _, err := os.Stat(jsonPath); os.IsNotExist(err) {
		return fmt.Errorf("rule set '%s' not found", tag)
	}

	cmd := exec.Command(binaryName, "rule-set", "compile", "--output", srsPath, jsonPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("compile failed: %s — %s", err, strings.TrimSpace(string(output)))
	}

	return nil
}

// writeSetUnlocked writes a rule set source to disk (caller must hold lock)
func (m *Manager) writeSetUnlocked(tag string, src *RuleSetSource) error {
	data, err := json.MarshalIndent(src, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal rule set: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(m.jsonPath(tag), data, 0644); err != nil {
		return fmt.Errorf("failed to write rule set: %w", err)
	}
	return nil
}

// validateDomain normalizes and validates a domain string
func validateDomain(raw string) (string, error) {
	domain := strings.ToLower(strings.TrimSpace(raw))

	// Reject empty
	if domain == "" {
		return "", fmt.Errorf("domain cannot be empty")
	}

	// Strip protocol prefixes (user might paste URLs)
	for _, prefix := range []string{"https://", "http://", "www."} {
		domain = strings.TrimPrefix(domain, prefix)
	}

	// Strip trailing path/query
	if idx := strings.IndexAny(domain, "/?#"); idx >= 0 {
		domain = domain[:idx]
	}

	// Strip trailing dot
	domain = strings.TrimRight(domain, ".")

	if !domainRegex.MatchString(domain) {
		return "", fmt.Errorf("invalid domain: '%s'", domain)
	}

	// Minimum: at least one dot for a real domain, or single-label is allowed
	if len(domain) > 253 {
		return "", fmt.Errorf("domain too long: '%s'", domain)
	}

	return domain, nil
}
