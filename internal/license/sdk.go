package license

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net"
	"strings"
	"sync"
	"time"
)

// LicenseSDK is the interface for server-side license verification
type LicenseSDK interface {
	// VerifyLocal verifies the local signed license file using the embedded public key
	VerifyLocal() (*LicenseInfo, error)

	// VerifyOnline performs an online license verification against the server
	VerifyOnline() (*LicenseInfo, error)

	// IsValid returns whether the license is currently valid (uses cache)
	IsValid() bool

	// GetEdition returns the current edition (community/professional/enterprise)
	GetEdition() string

	// CheckFeature checks if a feature is enabled
	CheckFeature(feature string) bool

	// GetLimit returns the value of a numeric limit, or 0 if not set.
	// Falls back to top-level fields (MaxUsers, MaxNodes, MaxEndUsers, MaxTenants)
	// when a well-known key matches.
	GetLimit(key string) int

	// GetInfo returns the cached license info
	GetInfo() *LicenseInfo

	// Start performs startup verification and begins background random checks
	// Flow: verify local file → verify online → update local config → random re-checks
	Start(ctx context.Context) error

	// Stop halts background verification
	Stop()

	// ── added for full-chain enforcement ──

	// Reload re-runs VerifyLocal + VerifyOnline without restarting the
	// process. Used after the operator swaps in a fresh license.json or
	// after the license server pushes a revocation notice.
	Reload() error

	// ReportUsage sends a single heartbeat carrying current usage so the
	// license server can reconcile contractual limits with actual
	// consumption. Best-effort: a transient network failure is logged
	// and swallowed (the next call will retry).
	ReportUsage(usage UsageReport)

	// SetUsageReporter wires an implementation that can answer
	// "what is the current b-tier usage". When set, the background loop
	// will call it before each periodic ReportUsage. Lets license stay
	// in its own package without depending on the db package.
	SetUsageReporter(r UsageReporter)

	// CheckBinding compares the current server fingerprint / hostname /
	// IP against the license's BoundServerIDs / BoundHostnames / BoundIPs.
	// Returns (ok, reason). An empty bound list = not bound = always ok.
	CheckBinding() (bool, string)

	// InvalidReason returns a human-readable explanation when IsValid()
	// is false. Empty string when the license is currently valid.
	InvalidReason() string

	// GetUsageStatus combines license limits with the most recently
	// reported usage so admin UI can render bars without a second round
	// trip to the license server. Returns zero-value when no usage has
	// been reported yet.
	GetUsageStatus() UsageStatus
}

// licenseSDK implements LicenseSDK
type licenseSDK struct {
	config   Config
	client   *httpClient
	cache    *cache
	serverID string
	vendorID string

	mu            sync.RWMutex
	info          *LicenseInfo
	valid         bool
	invalidReason string
	lastUsage     UsageReport
	reporter      UsageReporter
	cancel        context.CancelFunc
}

// New creates a new LicenseSDK instance
func New(cfg Config) LicenseSDK {
	// Apply defaults
	if cfg.CheckMinInterval == 0 {
		cfg.CheckMinInterval = 12 * time.Hour
	}
	if cfg.CheckMaxInterval == 0 {
		cfg.CheckMaxInterval = 72 * time.Hour
	}
	if cfg.GracePeriod == 0 {
		cfg.GracePeriod = 7 * 24 * time.Hour
	}
	if cfg.CacheFile == "" {
		cfg.CacheFile = ".license_cache"
	}
	if cfg.LicenseFile == "" {
		cfg.LicenseFile = "license.json"
	}

	sdk := &licenseSDK{
		config:   cfg,
		client:   newHTTPClient(cfg.ServerURL),
		cache:    newCache(cfg.CacheFile, cfg.LicenseID, cfg.PublicKey),
		serverID: GenerateServerID(),
	}

	// Try to load from cache on init
	if cached := sdk.cache.Get(); cached != nil {
		sdk.info = cached
		sdk.valid = true
	}

	return sdk
}

// VerifyLocal verifies the local signed license file
func (s *licenseSDK) VerifyLocal() (*LicenseInfo, error) {
	if s.config.PublicKey == "" {
		return nil, fmt.Errorf("public key is not configured")
	}

	info, err := VerifyLocalFile(s.config.LicenseFile, s.config.PublicKey)
	if err != nil {
		return nil, err
	}

	// Serial rollback detection: a license whose serial is strictly
	// lower than the highest serial this host has ever seen indicates
	// an attempt to revert by dropping back an old license.json file.
	// This is one of the few attacks that can succeed even against the
	// signed-file check (the old file is genuinely signed). The cache
	// stores the high-water mark under HMAC so it can't be reset by
	// the same attacker without also forging a fresh cache.
	if info.LicenseSerial > 0 {
		maxSeen := s.cache.MaxSeenSerial()
		if maxSeen > 0 && info.LicenseSerial < maxSeen {
			s.setInvalidWithReason(fmt.Sprintf(
				"license serial rollback detected: file=%d, last seen=%d",
				info.LicenseSerial, maxSeen))
			return nil, fmt.Errorf("license serial rollback: %d < %d",
				info.LicenseSerial, maxSeen)
		}
		s.cache.UpdateSerial(info.LicenseSerial)
	}

	// Validate product line — fail-closed on empty. The platform signs every
	// product line with the same key, so product_line is the only cross-product
	// barrier: an empty expected or licensed product line MUST be rejected, not
	// skipped, or a license issued for another product would validate here.
	if s.config.ProductLine == "" {
		return nil, fmt.Errorf("expected product line not configured: refusing to validate license")
	}
	if info.ProductLine == "" {
		return nil, fmt.Errorf("license has no product line: refusing untrusted license")
	}
	if info.ProductLine != s.config.ProductLine {
		return nil, fmt.Errorf("license product line mismatch: expected %s, got %s", s.config.ProductLine, info.ProductLine)
	}

	// Override LicenseID from config if set (file may contain it too)
	if s.config.LicenseID == "" && info.LicenseID != "" {
		s.config.LicenseID = info.LicenseID
		// Re-derive HMAC key now that we know the license ID（本地文件无 cache_secret）
		s.cache.ReinitKey(info.LicenseID, s.config.PublicKey, info.CacheSecret)
	}

	// Extract VendorID from signed file
	if info.VendorID != "" {
		s.vendorID = info.VendorID
	}

	// Override ServerURL from file if not set in config
	if s.config.ServerURL == "" && info.ServerURL != "" {
		s.config.ServerURL = info.ServerURL
		s.client = newHTTPClient(info.ServerURL)
	}

	s.mu.Lock()
	s.info = info
	s.valid = true
	s.mu.Unlock()

	s.cache.Set(info, s.config.GracePeriod)
	log.Printf("[license-sdk] local license file verified successfully (edition: %s, expires: %s)", info.Edition, info.ExpiresAt)
	return info, nil
}

// VerifyOnline performs an online license verification
func (s *licenseSDK) VerifyOnline() (*LicenseInfo, error) {
	if s.config.LicenseID == "" {
		return nil, fmt.Errorf("license_id is not configured")
	}
	if s.config.ServerURL == "" {
		return nil, fmt.Errorf("server_url is not configured")
	}

	req := &VerifyRequest{
		LicenseID:  s.config.LicenseID,
		VendorID:   s.getVendorID(),
		ServerID:   s.serverID,
		Hostname:   GetHostname(),
		OSInfo:     GetOSInfo(),
		AppVersion: s.config.AppVersion,
		Timestamp:  time.Now().Unix(),
	}

	resp, err := s.client.Verify(req)
	if err != nil {
		// Network error - use cached state if within grace period
		if s.cache.IsValid() {
			log.Printf("[license-sdk] online verification failed, using cached state: %v", err)
			return s.cache.Get(), nil
		}
		s.setInvalid()
		return nil, fmt.Errorf("license verification failed and cache expired: %w", err)
	}

	if resp.Data.Status == "valid" && resp.Data.License != nil {
		info := resp.Data.License
		info.VerifiedAt = time.Now()

		// Verify server response signature (anti-MITM)
		if s.config.PublicKey != "" {
			if resp.Data.Signature == "" {
				// Public key configured but response is unsigned: possible MITM / plaintext
				// hijack. Reject without trusting it, but do NOT setInvalid — tearing down a
				// locally-verified valid license on a single unsigned response is a DoS vector
				// (an attacker who can inject unsigned replies could force a downgrade).
				// The caller keeps the existing valid state; only an absence of any valid
				// credential degrades to community.
				return nil, fmt.Errorf("server response missing signature: refusing unsigned license update")
			} else if err := verifySignature(s.config.PublicKey, info, resp.Data.Signature); err != nil {
				s.setInvalid()
				return nil, fmt.Errorf("server response signature verification failed: %w", err)
			}
		}

		// Record server time for clock rollback detection
		if resp.Data.ServerTime > 0 {
			s.cache.UpdateServerTime(resp.Data.ServerTime)
		}

		// Update vendorID from server response
		if info.VendorID != "" {
			s.vendorID = info.VendorID
		}

		// Validate product line — fail-closed on empty (see VerifyLocal).
		if s.config.ProductLine == "" {
			s.setInvalid()
			return nil, fmt.Errorf("expected product line not configured: refusing to validate license")
		}
		if info.ProductLine == "" {
			s.setInvalid()
			return nil, fmt.Errorf("license has no product line: refusing untrusted license")
		}
		if info.ProductLine != s.config.ProductLine {
			s.setInvalid()
			return nil, fmt.Errorf("license product line mismatch: expected %s, got %s", s.config.ProductLine, info.ProductLine)
		}

		// 用服务端下发的 cache_secret 重派生缓存 HMAC key，使本地缓存完整性绑定该 secret
		// （否则 key 仅由公开公钥派生，持有公钥即可伪造/重放缓存）。须在写 serial/Set 之前，
		// 让后续写入都用新 key。secret 仅驻留内存、不持久化。
		if info.CacheSecret != "" {
			s.cache.SetSecret(info.LicenseID, s.config.PublicKey, info.CacheSecret)
		}

		// Serial rollback detection: server-issued license must not
		// regress below the highest serial this host has seen. Catches
		// the case where a license server is restored from old backup.
		if info.LicenseSerial > 0 {
			maxSeen := s.cache.MaxSeenSerial()
			if maxSeen > 0 && info.LicenseSerial < maxSeen {
				s.setInvalidWithReason(fmt.Sprintf(
					"license serial rollback from server: %d < last seen %d",
					info.LicenseSerial, maxSeen))
				return nil, fmt.Errorf("license serial rollback: %d < %d",
					info.LicenseSerial, maxSeen)
			}
			s.cache.UpdateSerial(info.LicenseSerial)
		}

		s.mu.Lock()
		s.info = info
		s.valid = true
		s.invalidReason = ""
		s.mu.Unlock()

		s.cache.Set(info, s.config.GracePeriod)

		// Update local license file with latest info from server
		s.updateLocalFromServer(info)

		log.Printf("[license-sdk] online verification successful (edition: %s, product: %s)", info.Edition, info.ProductLine)
		return info, nil
	}

	// License is not valid — pass the server-side status through to
	// InvalidReason so admin UI can render the actual cause:
	//   "expired" / "suspended" / "revoked" / "node_limit_exceeded"
	//   (MaxServers / MaxNodes 超限) / "domain_mismatch"
	// instead of a generic "verification failed".
	reason := fmt.Sprintf("license %s", resp.Data.Status)
	if resp.Data.Message != "" {
		reason += ": " + resp.Data.Message
	}
	s.setInvalidWithReason(reason)
	return nil, fmt.Errorf("%s", reason)
}

// Start performs startup verification and begins background random checks
func (s *licenseSDK) Start(ctx context.Context) error {
	// Warn if no public key is configured — signatures cannot be verified
	if s.config.PublicKey == "" {
		log.Printf("[license-sdk] WARNING: no public key configured — response signatures will NOT be verified. " +
			"Embed the public key at compile time using go:embed for tamper resistance.")
	}

	// Step 1: Verify local signed license file (if public key and file exist)
	if s.config.PublicKey != "" {
		localInfo, err := s.VerifyLocal()
		if err != nil {
			log.Printf("[license-sdk] local verification failed: %v", err)
		} else {
			log.Printf("[license-sdk] local license verified: %s (edition: %s)", localInfo.LicenseID, localInfo.Edition)
		}
	}

	// Step 2: Online verification (first startup must succeed unless we have valid local/cache)
	if s.config.ServerURL != "" && s.config.LicenseID != "" {
		_, err := s.VerifyOnline()
		if err != nil {
			if s.IsValid() {
				log.Printf("[license-sdk] online verification failed but local license is valid: %v", err)
			} else {
				return fmt.Errorf("license verification failed: %w", err)
			}
		}
	} else if !s.IsValid() {
		return fmt.Errorf("license is not configured (no license_id or server_url)")
	}

	// Step 2.5: Enforce bound server / hostname after verification.
	// Done after VerifyLocal/Online so info is populated. Fails closed
	// — a bound license on the wrong host must not boot.
	if ok, reason := s.CheckBinding(); !ok {
		s.setInvalidWithReason(reason)
		return fmt.Errorf("license binding check failed: %s", reason)
	}

	// Step 3: Start background random verification + usage heartbeat
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	go s.backgroundLoop(ctx)

	return nil
}

// Stop halts background verification
func (s *licenseSDK) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
}

// IsValid returns whether the license is currently valid
func (s *licenseSDK) IsValid() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.valid
}

// GetEdition returns the current edition
func (s *licenseSDK) GetEdition() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.valid || s.info == nil || s.info.Edition == "" {
		return "community"
	}
	return s.info.Edition
}

// CheckFeature checks if a feature is enabled
func (s *licenseSDK) CheckFeature(feature string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.valid || s.info == nil || s.info.Features == nil {
		return false
	}
	val, ok := s.info.Features[feature]
	if !ok {
		return false
	}
	switch v := val.(type) {
	case bool:
		return v
	case float64:
		return v > 0
	default:
		return true
	}
}

// GetLimit returns the value of a numeric limit. Looks first in the
// Limits map, then falls back to top-level fields for well-known keys.
// Returns 0 when not set (treated by call sites as "unlimited" or
// "not configured" — depends on context).
func (s *licenseSDK) GetLimit(key string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.valid || s.info == nil {
		return 0
	}
	if s.info.Limits != nil {
		if val, ok := s.info.Limits[key]; ok {
			if v, ok := val.(float64); ok {
				return int(v)
			}
		}
	}
	// Fallback: well-known keys mapped to top-level fields.
	switch key {
	case "max_users", "MaxUsers":
		return s.info.MaxUsers
	case "max_nodes", "MaxNodes":
		return s.info.MaxNodes
	case "max_servers", "MaxServers":
		return s.info.MaxServers
	case "max_end_users", "MaxEndUsers":
		return s.info.MaxEndUsers
	case "max_tenants", "MaxTenants":
		return s.info.MaxTenants
	}
	return 0
}

// GetInfo returns the cached license info
func (s *licenseSDK) GetInfo() *LicenseInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.info
}

func (s *licenseSDK) backgroundLoop(ctx context.Context) {
	// Usage heartbeat runs on a fixed 1h tick — verify runs on the
	// existing random 12-72h tick. Two separate timers avoid coupling
	// "tell server how much we used" to "ask server if we're still valid".
	verifyTimer := time.NewTimer(s.randomInterval())
	defer verifyTimer.Stop()
	usageTicker := time.NewTicker(time.Hour)
	defer usageTicker.Stop()
	// Fire one usage heartbeat soon after startup (3 min) so the license
	// server quickly learns this server exists.
	initialUsage := time.NewTimer(3 * time.Minute)
	defer initialUsage.Stop()

	reportNow := func() {
		s.mu.RLock()
		r := s.reporter
		s.mu.RUnlock()
		if r == nil {
			return
		}
		s.ReportUsage(r.CurrentUsage())
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-verifyTimer.C:
			if _, err := s.VerifyOnline(); err != nil {
				log.Printf("[license-sdk] background verification failed: %v", err)
			}
			verifyTimer.Reset(s.randomInterval())
		case <-initialUsage.C:
			reportNow()
		case <-usageTicker.C:
			reportNow()
			// Offline / no-server deployments never flip s.valid via VerifyOnline (it
			// early-returns). IsValid() returns the in-memory s.valid and does not consult
			// cache.Get()'s expiry check, so a license that expires while running offline
			// would stay "valid" until restart. Re-check the license's own expiry hourly.
			s.checkLocalExpiry()
		}
	}
}

// checkLocalExpiry actively invalidates the in-memory license once its own
// ExpiresAt (or trial cutoff) has passed, independent of online verification.
func (s *licenseSDK) checkLocalExpiry() {
	s.mu.RLock()
	info := s.info
	valid := s.valid
	s.mu.RUnlock()
	if !valid || info == nil {
		return
	}
	now := time.Now()
	expired := false
	if info.ExpiresAt != "" {
		if exp, err := time.Parse(time.RFC3339, info.ExpiresAt); err == nil && now.After(exp) {
			expired = true
		}
	}
	if !expired && info.IsTrial && info.TrialEndsAt != "" {
		if te, err := time.Parse(time.RFC3339, info.TrialEndsAt); err == nil && now.After(te) {
			expired = true
		}
	}
	if expired {
		s.setInvalidWithReason("license expired (offline self-check)")
	}
}

func (s *licenseSDK) randomInterval() time.Duration {
	min := s.config.CheckMinInterval
	max := s.config.CheckMaxInterval
	if max <= min {
		return min
	}
	delta := max - min
	return min + time.Duration(rand.Int63n(int64(delta)))
}

func (s *licenseSDK) setInvalid() {
	s.setInvalidWithReason("license verification failed")
}

func (s *licenseSDK) setInvalidWithReason(reason string) {
	s.mu.Lock()
	wasValid := s.valid
	s.valid = false
	s.invalidReason = reason
	info := s.info
	s.mu.Unlock()

	s.cache.Clear()

	if wasValid && s.config.OnInvalid != nil {
		s.config.OnInvalid(info)
	}
}

// Reload re-runs local + online verification without restarting.
func (s *licenseSDK) Reload() error {
	if _, err := s.VerifyLocal(); err != nil {
		log.Printf("[license-sdk] reload: local verify failed: %v", err)
	}
	if s.config.ServerURL != "" && s.config.LicenseID != "" {
		if _, err := s.VerifyOnline(); err != nil {
			if !s.IsValid() {
				return fmt.Errorf("reload failed and no valid cache: %w", err)
			}
			log.Printf("[license-sdk] reload: online verify failed but cache valid: %v", err)
		}
	}
	s.mu.Lock()
	if s.valid {
		s.invalidReason = ""
	}
	s.mu.Unlock()
	return nil
}

// SetUsageReporter wires a usage source. Safe to call before or after Start.
func (s *licenseSDK) SetUsageReporter(r UsageReporter) {
	s.mu.Lock()
	s.reporter = r
	s.mu.Unlock()
}

// ReportUsage sends a heartbeat with the given usage payload. Best-effort.
func (s *licenseSDK) ReportUsage(usage UsageReport) {
	if s.config.ServerURL == "" || s.config.LicenseID == "" {
		return
	}
	usage.ReportedAt = time.Now().Unix()
	req := &HeartbeatRequest{
		LicenseID: s.config.LicenseID,
		VendorID:  s.getVendorID(),
		ServerID:  s.serverID,
		Timestamp: usage.ReportedAt,
	}
	// Cache last usage so GetUsageStatus has something to render even
	// if the heartbeat HTTP call fails.
	s.mu.Lock()
	s.lastUsage = usage
	s.mu.Unlock()
	resp, err := s.client.HeartbeatWithUsage(req, usage)
	if err != nil {
		log.Printf("[license-sdk] ReportUsage failed (cached locally): %v", err)
		return
	}
	if resp != nil && resp.Data.Status == "invalid" {
		s.setInvalidWithReason("heartbeat: " + resp.Data.Message)
	}
}

// CheckBinding compares the current server fingerprint/hostname/IP
// against the license's bound lists. Empty bound lists = always ok.
func (s *licenseSDK) CheckBinding() (bool, string) {
	s.mu.RLock()
	info := s.info
	serverID := s.serverID
	s.mu.RUnlock()
	if info == nil {
		return true, ""
	}
	if len(info.BoundServerIDs) > 0 {
		matched := false
		for _, id := range info.BoundServerIDs {
			if id == serverID {
				matched = true
				break
			}
		}
		if !matched {
			return false, fmt.Sprintf("server_id %s not in license bound list", serverID)
		}
	}
	if len(info.BoundHostnames) > 0 {
		host := GetHostname()
		matched := false
		for _, h := range info.BoundHostnames {
			if h == host {
				matched = true
				break
			}
		}
		if !matched {
			return false, fmt.Sprintf("hostname %s not in license bound list", host)
		}
	}
	// BoundIPs: opt-in soft check. NAT environments have unstable
	// outbound IPs, so SDK can only do a best-effort probe at startup
	// and compare against the license's CIDR list. Operators who need
	// a hard guarantee should still configure network-layer firewalling
	// — this check catches the "license was meant for IP block X but
	// got deployed somewhere else entirely" case, not sub-IP drift.
	if len(info.BoundIPs) > 0 {
		probedIP := outboundIP()
		if probedIP == "" {
			// Probe failure shouldn't break a license — log and pass.
			// (Without internet to probe, BoundIPs is a moot question.)
			return true, ""
		}
		ip := net.ParseIP(probedIP)
		if ip == nil {
			return true, ""
		}
		for _, cidr := range info.BoundIPs {
			cidr = strings.TrimSpace(cidr)
			if cidr == "" {
				continue
			}
			// Single-IP entry vs CIDR
			if !strings.Contains(cidr, "/") {
				if cidr == probedIP {
					return true, ""
				}
				continue
			}
			_, ipnet, err := net.ParseCIDR(cidr)
			if err != nil {
				continue
			}
			if ipnet.Contains(ip) {
				return true, ""
			}
		}
		return false, fmt.Sprintf("outbound IP %s not in license bound list", probedIP)
	}
	return true, ""
}

// outboundIP returns the local IPv4 used to reach the public internet.
// Uses a UDP socket "connect" trick — no packets are sent. Returns ""
// when no network is reachable (probe should be ignored, not fail).
func outboundIP() string {
	c, err := net.DialTimeout("udp4", "8.8.8.8:53", 2*time.Second)
	if err != nil {
		return ""
	}
	defer c.Close()
	if addr, ok := c.LocalAddr().(*net.UDPAddr); ok {
		return addr.IP.String()
	}
	return ""
}

// InvalidReason returns why IsValid() == false. Empty when valid.
func (s *licenseSDK) InvalidReason() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.valid {
		return ""
	}
	if s.invalidReason != "" {
		return s.invalidReason
	}
	return "license invalid"
}

// GetUsageStatus combines license limits with last reported usage.
func (s *licenseSDK) GetUsageStatus() UsageStatus {
	s.mu.RLock()
	info := s.info
	usage := s.lastUsage
	s.mu.RUnlock()
	st := UsageStatus{
		UsersUsed:      usage.Users,
		EndUsersUsed:   usage.EndUsers,
		TenantsUsed:    usage.Tenants,
		HBBSNodesUsed:  usage.HBBSNodes,
		HBBRNodesUsed:  usage.HBBRNodes,
	}
	if info != nil {
		st.UsersLimit = info.MaxUsers
		st.EndUsersLimit = info.MaxEndUsers
		st.TenantsLimit = info.MaxTenants
		st.HBBSNodesLimit = info.MaxNodes
		st.HBBRNodesLimit = info.MaxNodes
		if info.ExpiresAt != "" {
			if t, err := time.Parse(time.RFC3339, info.ExpiresAt); err == nil {
				st.ExpiresInDays = int(time.Until(t).Hours() / 24)
				if st.ExpiresInDays < 0 {
					st.GracePeriodOpen = true
				}
			}
		}
	}
	return st
}

// getVendorID returns vendor ID with priority: cached from license file/server > config fallback
func (s *licenseSDK) getVendorID() string {
	if s.vendorID != "" {
		return s.vendorID
	}
	return s.config.VendorID
}

// updateLocalFromServer updates the local license cache file with latest info from server
func (s *licenseSDK) updateLocalFromServer(info *LicenseInfo) {
	if s.config.CacheFile == "" {
		return
	}

	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		log.Printf("[license-sdk] failed to serialize license info for cache: %v", err)
		return
	}

	s.cache.Set(info, s.config.GracePeriod)
	_ = data
}
