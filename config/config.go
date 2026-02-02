package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/spf13/viper"
)

type Config struct {
	Server     ServerConfig     `mapstructure:"server"`
	Database   DatabaseConfig   `mapstructure:"database"`
	JWT        JWTConfig        `mapstructure:"jwt"`
	Storage    StorageConfig    `mapstructure:"storage"`
	Blockchain BlockchainConfig `mapstructure:"blockchain"`
	Rate       RateConfig       `mapstructure:"rate"`
	Security   SecurityConfig   `mapstructure:"security"`
	Notify     NotifyConfig     `mapstructure:"notify"`
	Order      OrderConfig      `mapstructure:"order"`
	Log        LogConfig        `mapstructure:"log"`
}

type StorageConfig struct {
	DataDir string `mapstructure:"data_dir"` // 数据存储目录 (上传文件等)
}

type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

type DatabaseConfig struct {
	Host            string `mapstructure:"host"`
	Port            int    `mapstructure:"port"`
	User            string `mapstructure:"user"`
	Password        string `mapstructure:"password"`
	DBName          string `mapstructure:"dbname"`
	MaxOpenConns    int    `mapstructure:"max_open_conns"`    // 最大打开连接数
	MaxIdleConns    int    `mapstructure:"max_idle_conns"`    // 最大空闲连接数
	ConnMaxLifetime int    `mapstructure:"conn_max_lifetime"` // 连接最大生命周期(分钟)
}

type JWTConfig struct {
	Secret     string `mapstructure:"secret"`
	ExpireHour int    `mapstructure:"expire_hour"` // Token过期时间(小时)
}

// SecurityConfig 安全配置
type SecurityConfig struct {
	// 限流配置
	RateLimitAPI      float64 `mapstructure:"rate_limit_api"`       // API每秒请求数
	RateLimitAPIBurst int     `mapstructure:"rate_limit_api_burst"` // API突发容量
	RateLimitLogin    float64 `mapstructure:"rate_limit_login"`     // 登录每秒请求数
	RateLimitBurst    int     `mapstructure:"rate_limit_login_burst"`
	// CORS配置
	CORSAllowOrigins []string `mapstructure:"cors_allow_origins"` // 允许的来源域名
	// IP黑名单缓存
	IPBlacklistCacheTTL int `mapstructure:"ip_blacklist_cache_ttl"` // IP黑名单缓存时间(秒)
	// HTTP超时
	HTTPTimeout int `mapstructure:"http_timeout"` // 外部HTTP请求超时(秒)
}

// NotifyConfig 通知配置
type NotifyConfig struct {
	RetryCount    int `mapstructure:"retry_count"`    // 通知重试次数
	RetryInterval int `mapstructure:"retry_interval"` // 重试间隔(秒)
	Timeout       int `mapstructure:"timeout"`        // 通知超时(秒)
}

// OrderConfig 订单配置
type OrderConfig struct {
	ExpireMinutes   int `mapstructure:"expire_minutes"`   // 订单过期时间(分钟)
	CleanupHours    int `mapstructure:"cleanup_hours"`    // 自动清理多少小时前的无效订单
	WalletCacheTTL  int `mapstructure:"wallet_cache_ttl"` // 钱包缓存时间(秒)
}

// LogConfig 日志配置
type LogConfig struct {
	Level       string `mapstructure:"level"`        // 日志级别: debug, info, warn, error
	DBLogLevel  string `mapstructure:"db_log_level"` // 数据库日志级别
	APILogDays  int    `mapstructure:"api_log_days"` // API日志保留天数
}

type BlockchainConfig struct {
	TRX       ChainConfig `mapstructure:"trx"`
	TRC20     ChainConfig `mapstructure:"trc20"`
	ERC20     ChainConfig `mapstructure:"erc20"`
	BEP20     ChainConfig `mapstructure:"bep20"`
	Polygon   ChainConfig `mapstructure:"polygon"`
	Optimism  ChainConfig `mapstructure:"optimism"`
	Arbitrum  ChainConfig `mapstructure:"arbitrum"`
	Avalanche ChainConfig `mapstructure:"avalanche"`
	Base      ChainConfig `mapstructure:"base"`
}

type ChainConfig struct {
	Enabled         bool   `mapstructure:"enabled"`
	RPC             string `mapstructure:"rpc"`
	ContractAddress string `mapstructure:"contract_address"`
	Confirmations   int    `mapstructure:"confirmations"`
	ScanInterval    int    `mapstructure:"scan_interval"`
}

type RateConfig struct {
	AutoUpdateEnabled bool   `mapstructure:"auto_update_enabled"` // 是否启用自动更新
	UpdateInterval    int    `mapstructure:"update_interval"`     // 自动更新间隔(分钟)
	Source            string `mapstructure:"source"`              // 主数据源: binance, okx, custom
	FallbackSource    string `mapstructure:"fallback_source"`     // 备用数据源
	CustomAPI         string `mapstructure:"custom_api"`          // 自定义API地址
	CnyAPI            string `mapstructure:"cny_api"`             // CNY汇率专用API地址
	CacheSeconds      int    `mapstructure:"cache_seconds"`       // 汇率缓存时间(秒)
}

var cfg *Config

// getExeDir 获取可执行文件所在目录
func getExeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

func Load() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	// 获取可执行文件所在目录
	exeDir := getExeDir()

	// 按优先级添加配置路径
	viper.AddConfigPath(exeDir)        // 可执行文件所在目录 (开发/部署环境)
	viper.AddConfigPath(".")           // 当前工作目录
	viper.AddConfigPath("./config")    // 当前目录下的config目录
	viper.AddConfigPath("/etc/ezpay")  // 系统配置目录 (生产环境)

	// 设置默认值
	setDefaults()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// 配置文件不存在，创建默认配置
			if err := createDefaultConfig(); err != nil {
				return nil, fmt.Errorf("failed to create default config: %w", err)
			}
			if err := viper.ReadInConfig(); err != nil {
				return nil, fmt.Errorf("failed to read config: %w", err)
			}
		} else {
			return nil, fmt.Errorf("failed to read config: %w", err)
		}
	}

	cfg = &Config{}
	if err := viper.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return cfg, nil
}

func Get() *Config {
	return cfg
}

// getDefaultDataDir 根据平台返回默认数据目录
func getDefaultDataDir() string {
	switch runtime.GOOS {
	case "linux":
		return "/var/lib/ezpay"
	case "windows":
		// Windows: 当前目录/ezpay_data
		return filepath.Join(getExeDir(), "ezpay_data")
	default:
		// macOS 和其他: 当前目录/ezpay_data
		return filepath.Join(getExeDir(), "ezpay_data")
	}
}

func setDefaults() {
	// Server
	viper.SetDefault("server.host", "0.0.0.0")
	viper.SetDefault("server.port", 6088)

	// Database
	viper.SetDefault("database.host", "127.0.0.1")
	viper.SetDefault("database.port", 3306)
	viper.SetDefault("database.user", "ezpay")
	viper.SetDefault("database.password", "ezpay123")
	viper.SetDefault("database.dbname", "ezpay")
	viper.SetDefault("database.max_open_conns", 100)
	viper.SetDefault("database.max_idle_conns", 10)
	viper.SetDefault("database.conn_max_lifetime", 60) // 60分钟

	// JWT
	viper.SetDefault("jwt.secret", "change-this-secret-key-in-production")
	viper.SetDefault("jwt.expire_hour", 24)

	// Security
	viper.SetDefault("security.rate_limit_api", 20)
	viper.SetDefault("security.rate_limit_api_burst", 50)
	viper.SetDefault("security.rate_limit_login", 2)
	viper.SetDefault("security.rate_limit_login_burst", 5)
	viper.SetDefault("security.cors_allow_origins", []string{})
	viper.SetDefault("security.ip_blacklist_cache_ttl", 30)
	viper.SetDefault("security.http_timeout", 15)

	// Notify
	viper.SetDefault("notify.retry_count", 5)
	viper.SetDefault("notify.retry_interval", 60)
	viper.SetDefault("notify.timeout", 10)

	// Order
	viper.SetDefault("order.expire_minutes", 30)
	viper.SetDefault("order.cleanup_hours", 24)
	viper.SetDefault("order.wallet_cache_ttl", 30)

	// Log
	viper.SetDefault("log.level", "info")
	viper.SetDefault("log.db_log_level", "warn")
	viper.SetDefault("log.api_log_days", 30)

	// 存储目录 (根据平台自动设置默认值)
	viper.SetDefault("storage.data_dir", getDefaultDataDir())

	// TRX (Tron原生代币)
	viper.SetDefault("blockchain.trx.enabled", true)
	viper.SetDefault("blockchain.trx.rpc", "https://api.trongrid.io")
	viper.SetDefault("blockchain.trx.contract_address", "")
	viper.SetDefault("blockchain.trx.confirmations", 19)
	viper.SetDefault("blockchain.trx.scan_interval", 15)

	// TRC20 (Tron)
	viper.SetDefault("blockchain.trc20.enabled", true)
	viper.SetDefault("blockchain.trc20.rpc", "https://api.trongrid.io")
	viper.SetDefault("blockchain.trc20.contract_address", "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t")
	viper.SetDefault("blockchain.trc20.confirmations", 19)
	viper.SetDefault("blockchain.trc20.scan_interval", 15)

	// ERC20 (Ethereum)
	viper.SetDefault("blockchain.erc20.enabled", false)
	viper.SetDefault("blockchain.erc20.rpc", "https://mainnet.infura.io/v3/YOUR-PROJECT-ID")
	viper.SetDefault("blockchain.erc20.contract_address", "0xdAC17F958D2ee523a2206206994597C13D831ec7")
	viper.SetDefault("blockchain.erc20.confirmations", 12)
	viper.SetDefault("blockchain.erc20.scan_interval", 15)

	// BEP20 (BSC)
	viper.SetDefault("blockchain.bep20.enabled", true)
	viper.SetDefault("blockchain.bep20.rpc", "https://bsc-dataseed.binance.org")
	viper.SetDefault("blockchain.bep20.contract_address", "0x55d398326f99059fF775485246999027B3197955")
	viper.SetDefault("blockchain.bep20.confirmations", 15)
	viper.SetDefault("blockchain.bep20.scan_interval", 15)

	// Polygon
	viper.SetDefault("blockchain.polygon.enabled", true)
	viper.SetDefault("blockchain.polygon.rpc", "https://polygon-rpc.com")
	viper.SetDefault("blockchain.polygon.contract_address", "0xc2132D05D31c914a87C6611C10748AEb04B58e8F")
	viper.SetDefault("blockchain.polygon.confirmations", 128)
	viper.SetDefault("blockchain.polygon.scan_interval", 15)

	// Optimism
	viper.SetDefault("blockchain.optimism.enabled", true)
	viper.SetDefault("blockchain.optimism.rpc", "https://mainnet.optimism.io")
	viper.SetDefault("blockchain.optimism.contract_address", "0x94b008aA00579c1307B0EF2c499aD98a8ce58e58")
	viper.SetDefault("blockchain.optimism.confirmations", 10)
	viper.SetDefault("blockchain.optimism.scan_interval", 15)

	// Arbitrum
	viper.SetDefault("blockchain.arbitrum.enabled", true)
	viper.SetDefault("blockchain.arbitrum.rpc", "https://arb1.arbitrum.io/rpc")
	viper.SetDefault("blockchain.arbitrum.contract_address", "0xFd086bC7CD5C481DCC9C85ebE478A1C0b69FCbb9")
	viper.SetDefault("blockchain.arbitrum.confirmations", 10)
	viper.SetDefault("blockchain.arbitrum.scan_interval", 15)

	// Avalanche C-Chain
	viper.SetDefault("blockchain.avalanche.enabled", false)
	viper.SetDefault("blockchain.avalanche.rpc", "https://api.avax.network/ext/bc/C/rpc")
	viper.SetDefault("blockchain.avalanche.contract_address", "0x9702230A8Ea53601f5cD2dc00fDBc13d4dF4A8c7")
	viper.SetDefault("blockchain.avalanche.confirmations", 12)
	viper.SetDefault("blockchain.avalanche.scan_interval", 15)

	// Base
	viper.SetDefault("blockchain.base.enabled", true)
	viper.SetDefault("blockchain.base.rpc", "https://mainnet.base.org")
	viper.SetDefault("blockchain.base.contract_address", "0xfde4C96c8593536E31F229EA8f37b2ADa2699bb2")
	viper.SetDefault("blockchain.base.confirmations", 10)
	viper.SetDefault("blockchain.base.scan_interval", 15)

	// Rate
	viper.SetDefault("rate.mode", "hybrid")
	viper.SetDefault("rate.manual_rate", 7.2)
	viper.SetDefault("rate.float_percent", 0)
	viper.SetDefault("rate.cache_seconds", 300)
	viper.SetDefault("rate.source", "binance")
	viper.SetDefault("rate.cny_api", "https://api.exchangerate-api.com/v4/latest/USD")
}

func createDefaultConfig() error {
	// 根据平台生成默认数据目录
	dataDir := getDefaultDataDir()

	configContent := fmt.Sprintf(`# EzPay 配置文件
# 所有配置项都可在此文件中设置
# 管理员账号和Telegram配置在数据库/管理后台中管理

server:
  host: "0.0.0.0"
  port: 6088

database:
  host: "127.0.0.1"
  port: 3306
  user: "ezpay"
  password: "ezpay123"
  dbname: "ezpay"
  max_open_conns: 100
  max_idle_conns: 10
  conn_max_lifetime: 60

jwt:
  secret: "change-this-secret-key-in-production"
  expire_hour: 24

storage:
  data_dir: "%s"

security:
  rate_limit_api: 20
  rate_limit_api_burst: 50
  rate_limit_login: 2
  rate_limit_login_burst: 5
  cors_allow_origins: []
  ip_blacklist_cache_ttl: 30
  http_timeout: 15

order:
  expire_minutes: 30
  cleanup_hours: 24
  wallet_cache_ttl: 30

notify:
  retry_count: 5
  retry_interval: 60
  timeout: 10

log:
  level: "info"
  db_log_level: "warn"
  api_log_days: 30

rate:
  mode: "hybrid"
  manual_rate: 7.2
  float_percent: 0
  cache_seconds: 300
  source: "binance"

blockchain:
  trx:
    enabled: true
    rpc: "https://api.trongrid.io"
    contract_address: ""
    confirmations: 19
    scan_interval: 15
  trc20:
    enabled: true
    rpc: "https://api.trongrid.io"
    contract_address: "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t"
    confirmations: 19
    scan_interval: 15
  erc20:
    enabled: false
    rpc: "https://mainnet.infura.io/v3/YOUR-PROJECT-ID"
    contract_address: "0xdAC17F958D2ee523a2206206994597C13D831ec7"
    confirmations: 12
    scan_interval: 15
  bep20:
    enabled: true
    rpc: "https://bsc-dataseed.binance.org"
    contract_address: "0x55d398326f99059fF775485246999027B3197955"
    confirmations: 15
    scan_interval: 15
  polygon:
    enabled: true
    rpc: "https://polygon-rpc.com"
    contract_address: "0xc2132D05D31c914a87C6611C10748AEb04B58e8F"
    confirmations: 128
    scan_interval: 15
  optimism:
    enabled: true
    rpc: "https://mainnet.optimism.io"
    contract_address: "0x94b008aA00579c1307B0EF2c499aD98a8ce58e58"
    confirmations: 10
    scan_interval: 15
  arbitrum:
    enabled: true
    rpc: "https://arb1.arbitrum.io/rpc"
    contract_address: "0xFd086bC7CD5C481DCC9C85ebE478A1C0b69FCbb9"
    confirmations: 10
    scan_interval: 15
  avalanche:
    enabled: false
    rpc: "https://api.avax.network/ext/bc/C/rpc"
    contract_address: "0x9702230A8Ea53601f5cD2dc00fDBc13d4dF4A8c7"
    confirmations: 12
    scan_interval: 15
  base:
    enabled: true
    rpc: "https://mainnet.base.org"
    contract_address: "0xfde4C96c8593536E31F229EA8f37b2ADa2699bb2"
    confirmations: 10
    scan_interval: 15
`, dataDir)

	// 在可执行文件所在目录创建配置文件
	configPath := filepath.Join(getExeDir(), "config.yaml")
	return os.WriteFile(configPath, []byte(configContent), 0644)
}

func (c *DatabaseConfig) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		c.User, c.Password, c.Host, c.Port, c.DBName)
}
