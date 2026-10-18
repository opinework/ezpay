package license

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"sync"
	"time"
)

// cachedState holds the cached license state for offline operation
type cachedState struct {
	Info           *LicenseInfo `json:"info"`
	CachedAt       time.Time    `json:"cached_at"`
	ExpiresAt      time.Time    `json:"expires_at"`
	LastServerTime int64        `json:"last_server_time"` // last known server timestamp for clock rollback detection
	// LastSerial is the highest license_serial observed across all
	// successful verifications on this host. A subsequent license whose
	// serial is lower indicates a rollback attempt (operator dropped an
	// old license.json back into place to revert a downgrade).
	LastSerial int64  `json:"last_serial"`
	HMAC       string `json:"hmac"` // HMAC-SHA256 of the payload for tamper detection
}

// cache provides thread-safe in-memory + file-based license cache with integrity protection
type cache struct {
	mu       sync.RWMutex
	state    *cachedState
	filePath string
	hmacKey  []byte // derived from license_id + public_key + cache_secret
	// cacheSecret 是服务端在签名的在线验签响应里下发的 secret（仅驻留内存，绝不落盘）。
	// 落盘即可被攻击者连同 HMAC 一起伪造，等于没有保护。
	cacheSecret string
}

func newCache(filePath, licenseID, publicKey string) *cache {
	c := &cache{
		filePath: filePath,
		hmacKey:  deriveHMACKey(licenseID, publicKey, ""),
	}
	c.loadFromFile()
	return c
}

// deriveHMACKey creates an HMAC key from license identity material.
// 纳入 cacheSecret：空时退化为 license_id+public_key（向后兼容且缓存可跨重启加载）；
// 服务端下发非空 secret 后，缓存完整性 key 绑定该 secret——仅持有内嵌公钥的攻击者
// 无法伪造/重放 .license_cache（否则 key 仅由公开的公钥派生，形同虚设）。
func deriveHMACKey(licenseID, publicKey, cacheSecret string) []byte {
	h := sha256.Sum256([]byte(licenseID + "|" + publicKey + "|" + cacheSecret))
	return h[:]
}

// ReinitKey re-derives the HMAC key (used when licenseID is learned after init).
// 重载文件以恢复此前用同一 key 写过的缓存；本地文件无 cache_secret，调用方传空。
func (c *cache) ReinitKey(licenseID, publicKey, cacheSecret string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cacheSecret = cacheSecret
	c.hmacKey = deriveHMACKey(licenseID, publicKey, cacheSecret)
	// Reload file with new key (old data will fail HMAC and be discarded)
	c.loadFromFileUnlocked()
}

// SetSecret 在在线验签成功、学到/轮换服务端 cache_secret 后调用：用新 key 重派生，
// 并用新 key 重新签名当前内存态后落盘（不重载、不丢已记录的 serial/server_time）。
func (c *cache) SetSecret(licenseID, publicKey, cacheSecret string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	newKey := deriveHMACKey(licenseID, publicKey, cacheSecret)
	if hmac.Equal(newKey, c.hmacKey) {
		return // key 未变化，无需重写
	}
	c.cacheSecret = cacheSecret
	c.hmacKey = newKey
	if c.state != nil {
		c.saveToFile() // 用新 key 重新签名当前态
	}
}

// Get returns the cached license info if still valid
func (c *cache) Get() *LicenseInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.state == nil {
		return nil
	}
	// Clock rollback detection: if current time is significantly before last known server time,
	// treat cache as invalid (someone may have set clock back to extend grace period)
	if c.state.LastServerTime > 0 {
		now := time.Now().Unix()
		if now < c.state.LastServerTime-300 { // allow 5 min drift
			return nil
		}
	}
	if time.Now().After(c.state.ExpiresAt) {
		return nil
	}
	// License self-expiry is a HARD cutoff independent of the grace window.
	// The grace period only covers a transient inability to reach the license
	// server — it must NEVER extend a license past its own ExpiresAt / trial
	// cutoff. Without this, an attacker who simply blocks the license server
	// keeps an expired (or trial-ended) license "valid" for the whole grace
	// period offline, since the background loop only re-verifies online.
	if info := c.state.Info; info != nil {
		now := time.Now()
		if info.ExpiresAt != "" {
			if exp, err := time.Parse(time.RFC3339, info.ExpiresAt); err == nil && now.After(exp) {
				return nil
			}
		}
		if info.IsTrial && info.TrialEndsAt != "" {
			if te, err := time.Parse(time.RFC3339, info.TrialEndsAt); err == nil && now.After(te) {
				return nil
			}
		}
	}
	return c.state.Info
}

// Set updates the cache with new license info
func (c *cache) Set(info *LicenseInfo, gracePeriod time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.state = &cachedState{
		Info:           info,
		CachedAt:       time.Now(),
		ExpiresAt:      time.Now().Add(gracePeriod),
		LastServerTime: c.getLastServerTimeLocked(),
		// Carry the anti-rollback serial high-water mark forward. Without this
		// every Set() resets LastSerial to 0, so the high-water never survives
		// the next save/reboot and rollback detection silently does nothing.
		LastSerial: c.getLastSerialLocked(),
	}

	c.saveToFile()
}

// UpdateServerTime records the latest server time for clock rollback detection
func (c *cache) UpdateServerTime(serverTime int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.state != nil && serverTime > c.state.LastServerTime {
		c.state.LastServerTime = serverTime
		c.saveToFile()
	}
}

// MaxSeenSerial returns the highest license_serial this host has ever
// successfully verified. Returns 0 when nothing has been recorded yet
// (first-ever boot or after Clear()). Callers compare new license's
// serial against this value to detect rollback.
func (c *cache) MaxSeenSerial() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.state == nil {
		return 0
	}
	return c.state.LastSerial
}

// UpdateSerial records a newly observed license_serial. Monotonic: only
// advances, never goes backward. Persisted (under HMAC) so an attacker
// can't roll the file back by deleting just the serial entry.
func (c *cache) UpdateSerial(serial int64) {
	if serial <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state == nil {
		// No cached state yet (very first verification before Set()) —
		// stash it in a fresh state container; Set() will fold it in.
		c.state = &cachedState{LastSerial: serial}
		c.saveToFile()
		return
	}
	if serial > c.state.LastSerial {
		c.state.LastSerial = serial
		c.saveToFile()
	}
}

// Clear drops the cached grace/validity state (Info + expiry) but PRESERVES
// the monotonic anti-rollback high-water marks (LastSerial + LastServerTime).
// Wiping those on invalidation would let an attacker reset rollback protection
// for free: drop an old license → VerifyLocal flags a serial rollback →
// setInvalid → Clear → high-water gone → restart → the old license now passes.
// The serial / server-time counters must only ever advance, never be cleared.
func (c *cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	var serial, serverTime int64
	if c.state != nil {
		serial = c.state.LastSerial
		serverTime = c.state.LastServerTime
	}
	if serial > 0 || serverTime > 0 {
		// Keep a minimal state holding only the high-water marks, re-signed
		// under HMAC. ExpiresAt stays zero → Get() returns nil (invalid),
		// but MaxSeenSerial / clock-rollback detection survive.
		c.state = &cachedState{LastSerial: serial, LastServerTime: serverTime}
		c.saveToFile()
		return
	}
	c.state = nil
	if c.filePath != "" {
		os.Remove(c.filePath)
	}
}

// IsValid returns whether the cache contains valid data
func (c *cache) IsValid() bool {
	return c.Get() != nil
}

func (c *cache) getLastServerTimeLocked() int64 {
	if c.state != nil {
		return c.state.LastServerTime
	}
	return 0
}

func (c *cache) getLastSerialLocked() int64 {
	if c.state != nil {
		return c.state.LastSerial
	}
	return 0
}

func (c *cache) loadFromFile() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.loadFromFileUnlocked()
}

func (c *cache) loadFromFileUnlocked() {
	if c.filePath == "" {
		return
	}
	data, err := os.ReadFile(c.filePath)
	if err != nil {
		return
	}
	var state cachedState
	if err := json.Unmarshal(data, &state); err != nil {
		return
	}

	// Verify HMAC integrity
	if !c.verifyHMAC(&state) {
		// Cache has been tampered with — discard
		c.state = nil
		os.Remove(c.filePath)
		return
	}

	c.state = &state
}

func (c *cache) saveToFile() {
	if c.filePath == "" || c.state == nil {
		return
	}

	// Compute HMAC over the payload (excluding the HMAC field itself)
	c.state.HMAC = c.computeHMAC(c.state)

	data, err := json.Marshal(c.state)
	if err != nil {
		return
	}
	os.WriteFile(c.filePath, data, 0600)
}

// computeHMAC generates HMAC-SHA256 over the cache payload (info + timestamps)
func (c *cache) computeHMAC(state *cachedState) string {
	if len(c.hmacKey) == 0 {
		return ""
	}
	// Build canonical payload: info JSON + cached_at + expires_at + last_server_time
	payload := struct {
		Info           *LicenseInfo `json:"info"`
		CachedAt       time.Time    `json:"cached_at"`
		ExpiresAt      time.Time    `json:"expires_at"`
		LastServerTime int64        `json:"last_server_time"`
		LastSerial     int64        `json:"last_serial"`
	}{
		Info:           state.Info,
		CachedAt:       state.CachedAt,
		ExpiresAt:      state.ExpiresAt,
		LastServerTime: state.LastServerTime,
		// LastSerial MUST be covered by the HMAC: it is the anti-rollback
		// high-water mark. Left out, an attacker can edit the cache file to
		// lower last_serial without breaking the HMAC, then drop an old
		// (genuinely signed) license back in and defeat rollback detection.
		LastSerial: state.LastSerial,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	mac := hmac.New(sha256.New, c.hmacKey)
	mac.Write(data)
	return hex.EncodeToString(mac.Sum(nil))
}

// verifyHMAC checks that the cache file hasn't been tampered with
func (c *cache) verifyHMAC(state *cachedState) bool {
	if len(c.hmacKey) == 0 {
		// Fail-closed: an empty HMAC key must reject, never blanket-accept.
		// The key is always derived (sha256 is never empty) so this is
		// currently unreachable, but a blanket "return true" is a latent
		// fail-open that would trust any tampered cache file.
		return false
	}
	if state.HMAC == "" {
		return false // key is set but file has no HMAC = old format or tampered
	}
	expected := c.computeHMAC(state)
	return hmac.Equal([]byte(state.HMAC), []byte(expected))
}
