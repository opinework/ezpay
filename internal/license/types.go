package license

import "time"

// Config holds the SDK configuration
type Config struct {
	// LicenseID is the license key assigned to this server
	LicenseID string `yaml:"license_id" json:"license_id"`

	// ServerURL is the base URL of the license server (e.g., https://breez.work)
	ServerURL string `yaml:"server_url" json:"server_url"`

	// PublicKey is the RSA public key (PEM format) for signature verification.
	//
	// SECURITY: This key MUST be embedded at compile time using go:embed or a string constant.
	// Do NOT load from an external file at runtime — an attacker could substitute the key
	// to bypass license verification. Example:
	//
	//   //go:embed public_key.pem
	//   var publicKeyPEM string
	//
	//   cfg := license.Config{PublicKey: publicKeyPEM, ...}
	PublicKey string `yaml:"public_key" json:"public_key"`

	// LicenseFile is the path to the local signed license file (default "license.json")
	LicenseFile string `yaml:"license_file" json:"license_file"`

	// AppVersion is the current application version
	AppVersion string `yaml:"app_version" json:"app_version"`

	// VendorID is the vendor identifier for this server (fallback if not in signed license file)
	VendorID string `yaml:"vendor_id" json:"vendor_id"`

	// ProductLine is the expected product line code (e.g., "eztunnel", "ezpanel")
	// If set, the SDK will reject licenses that don't match this product line
	ProductLine string `yaml:"product_line" json:"product_line"`

	// CheckMinInterval is the minimum interval between random verification checks (default 12h)
	CheckMinInterval time.Duration `yaml:"check_min_interval" json:"check_min_interval"`

	// CheckMaxInterval is the maximum interval between random verification checks (default 72h)
	CheckMaxInterval time.Duration `yaml:"check_max_interval" json:"check_max_interval"`

	// GracePeriod is how long to allow operation if the server is unreachable (default 7 days)
	GracePeriod time.Duration `yaml:"grace_period" json:"grace_period"`

	// CacheFile is the path to the local cache file for offline operation
	CacheFile string `yaml:"cache_file" json:"cache_file"`

	// OnInvalid is called when the license becomes invalid
	OnInvalid func(info *LicenseInfo) `yaml:"-" json:"-"`
}

// LicenseInfo contains the cached license information
type LicenseInfo struct {
	LicenseID    string                 `json:"license_id"`
	VendorID     string                 `json:"vendor_id"`
	ProductLine  string                 `json:"product_line"`
	ProductCode  string                 `json:"product_code"`
	Edition      string                 `json:"edition"`
	ExpiresAt    string                 `json:"expires_at"`
	Features     map[string]interface{} `json:"features"`
	Limits       map[string]interface{} `json:"limits"`
	MaxNodes     int                    `json:"max_nodes"`
	MaxServers   int                    `json:"max_servers"`
	MaxUsers     int                    `json:"max_users"`
	BoundDomains []string               `json:"bound_domains,omitempty"`
	ServerURL    string                 `json:"server_url,omitempty"`
	// VerifiedAt is runtime-only (when this host last verified). Excluded from
	// JSON so it never enters the canonical/signed payload — the online verify
	// response signs ServerLicenseInfo, which has no such field; emitting it
	// here broke signature verification.
	VerifiedAt   time.Time              `json:"-"`

	// ── added for full-chain enforcement (see docs/10_licensing-quota.md) ──

	// MaxEndUsers caps how many c-tier accounts (admin_user.role='user')
	// the b-tier operator can serve. Independent of MaxUsers, which caps
	// back-office admins.
	MaxEndUsers int `json:"max_end_users,omitempty"`

	// MaxTenants caps the number of tenant rows the operator can create.
	MaxTenants int `json:"max_tenants,omitempty"`

	// BoundServerIDs locks the license to specific GenerateServerID()
	// fingerprints. Empty list = not bound.
	BoundServerIDs []string `json:"bound_server_ids,omitempty"`

	// BoundHostnames locks the license to specific OS hostnames.
	BoundHostnames []string `json:"bound_hostnames,omitempty"`

	// BoundIPs locks the license to specific source IP CIDRs.
	BoundIPs []string `json:"bound_ips,omitempty"`

	// IsTrial marks a trial license. TrialEndsAt is the hard cutoff;
	// trial licenses cannot be extended via grace period.
	IsTrial     bool   `json:"is_trial,omitempty"`
	TrialEndsAt string `json:"trial_ends_at,omitempty"`

	// LicenseSerial is monotonically increasing — used to detect cache
	// rollback (a copied old license.json) and to invalidate caches
	// during license server pushes.
	LicenseSerial int64 `json:"license_serial,omitempty"`

	// CacheSecret is the 32-byte secret carried in the signed online verify
	// response (ServerLicenseInfo.cache_secret). It MUST be represented here so
	// the SDK reproduces the exact canonical JSON the server signed — omitting
	// the field dropped it on re-marshal and broke signature verification.
	// Absent from the offline license file (deliberately not signed there).
	CacheSecret string `json:"cache_secret,omitempty"`
}

// SignedServerLicense matches the server-generated signed license file format
type SignedServerLicense struct {
	Signed    bool               `json:"__signed"`
	Version   string             `json:"__version"`
	Algorithm string             `json:"__algorithm"`
	SignedAt  string             `json:"__signed_at"`
	License   *ServerLicenseData `json:"license"`
	Signature string             `json:"signature"`
	PublicKey string             `json:"public_key"` // RSA public key PEM (embedded for self-contained verification)
}

// ServerLicenseData matches the license data section in the signed file
type ServerLicenseData struct {
	LicenseID    string                 `json:"licenseId"`
	ProductLine  string                 `json:"productLine"`
	ProductCode  string                 `json:"productCode"`
	VendorID     string                 `json:"vendorId"`
	VendorName   string                 `json:"vendorName"`
	Edition      string                 `json:"edition"`
	IssuedAt     string                 `json:"issuedAt"`
	ExpiresAt    string                 `json:"expiresAt"`
	MaxNodes     int                    `json:"maxNodes"`
	MaxServers   int                    `json:"maxServers"`
	MaxUsers     int                    `json:"maxUsers"`
	Features     map[string]interface{} `json:"features,omitempty"`
	Limits       map[string]interface{} `json:"limits,omitempty"`
	BoundDomains []string               `json:"boundDomains,omitempty"`
	ServerURL    string                 `json:"serverUrl"`
	// 签发端离线 license 的被签 payload 含 panelUrl（订阅配了面板域名时）；本端必须保留
	// 该字段，否则 canonicalJSON 缺 panelUrl 致离线验签必败（白标服务端授权起不来）。
	PanelURL     string                 `json:"panelUrl,omitempty"`

	// Mirror of LicenseInfo extension fields — kept in the signed payload
	// so the server can issue them as part of the signature.
	MaxEndUsers    int      `json:"maxEndUsers,omitempty"`
	MaxTenants     int      `json:"maxTenants,omitempty"`
	BoundServerIDs []string `json:"boundServerIds,omitempty"`
	BoundHostnames []string `json:"boundHostnames,omitempty"`
	BoundIPs       []string `json:"boundIps,omitempty"`
	IsTrial        bool     `json:"isTrial,omitempty"`
	TrialEndsAt    string   `json:"trialEndsAt,omitempty"`
	LicenseSerial  int64    `json:"licenseSerial,omitempty"`
}

// UsageReport is sent in periodic heartbeats so the license server can
// reconcile contractual limits with actual consumption. Empty zero-value
// fields are omitted from the JSON body.
type UsageReport struct {
	Users        int   `json:"users,omitempty"`         // back-office admins
	EndUsers     int   `json:"end_users,omitempty"`     // c-tier accounts
	Tenants      int   `json:"tenants,omitempty"`
	HBBSNodes    int   `json:"hbbs_nodes,omitempty"`
	HBBRNodes    int   `json:"hbbr_nodes,omitempty"`
	ActivePlans  int   `json:"active_plans,omitempty"`
	OnlinePeers  int64 `json:"online_peers,omitempty"`
	TotalDevices int64 `json:"total_devices,omitempty"`
	ReportedAt   int64 `json:"reported_at"`
}

// UsageStatus is what GetUsageStatus returns to admin UI — current usage
// alongside the limits the license imposes. Lets the UI render bars like
// "8 of 20 admins used".
type UsageStatus struct {
	UsersUsed       int `json:"users_used"`
	UsersLimit      int `json:"users_limit"`
	EndUsersUsed    int `json:"end_users_used"`
	EndUsersLimit   int `json:"end_users_limit"`
	TenantsUsed     int `json:"tenants_used"`
	TenantsLimit    int `json:"tenants_limit"`
	HBBSNodesUsed   int `json:"hbbs_nodes_used"`
	HBBSNodesLimit  int `json:"hbbs_nodes_limit"`
	HBBRNodesUsed   int `json:"hbbr_nodes_used"`
	HBBRNodesLimit  int `json:"hbbr_nodes_limit"`
	ExpiresInDays   int `json:"expires_in_days"`
	GracePeriodOpen bool `json:"grace_period_open"`
}

// UsageReporter is implemented by anything that can answer "how many of X
// are currently in use". The license SDK calls this before heartbeats so
// it doesn't need to depend on the DB package directly.
type UsageReporter interface {
	CurrentUsage() UsageReport
}

// VerifyRequest is sent to the license server
type VerifyRequest struct {
	LicenseID  string `json:"license_id"`
	VendorID   string `json:"vendor_id"`
	ServerID   string `json:"server_id"`
	Hostname   string `json:"hostname"`
	IPAddress  string `json:"ip_address"`
	OSInfo     string `json:"os_info"`
	AppVersion string `json:"app_version"`
	Timestamp  int64  `json:"timestamp"`
}

// VerifyResponse is received from the license server
type VerifyResponse struct {
	Code int `json:"code"`
	Data struct {
		Status         string       `json:"status"`
		Message        string       `json:"message,omitempty"`
		License        *LicenseInfo `json:"license,omitempty"`
		Signature      string       `json:"signature,omitempty"`
		NextCheckAfter int          `json:"next_check_after,omitempty"`
		ServerTime     int64        `json:"server_time,omitempty"`
	} `json:"data"`
}

// HeartbeatRequest is sent for periodic heartbeat
type HeartbeatRequest struct {
	LicenseID string `json:"license_id"`
	VendorID  string `json:"vendor_id"`
	ServerID  string `json:"server_id"`
	Timestamp int64  `json:"timestamp"`
}

// HeartbeatResponse is received from heartbeat
type HeartbeatResponse struct {
	Code int `json:"code"`
	Data struct {
		Status     string `json:"status"`
		ServerTime int64  `json:"server_time"`
		Message    string `json:"message,omitempty"`
	} `json:"data"`
}
