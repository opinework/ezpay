package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ezpay/config"
	"ezpay/internal/handler"
	"ezpay/internal/middleware"
	"ezpay/internal/model"
	"ezpay/internal/service"
	"ezpay/web"

	"github.com/gin-gonic/gin"
)

func main() {
	// 加载配置
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 初始化数据库（使用配置的连接池参数）
	dbConfig := model.DBConfig{
		MaxOpenConns:    cfg.Database.MaxOpenConns,
		MaxIdleConns:    cfg.Database.MaxIdleConns,
		ConnMaxLifetime: time.Duration(cfg.Database.ConnMaxLifetime) * time.Minute,
		ConnMaxIdleTime: 10 * time.Minute,
	}
	if err := model.InitDBWithConfig(cfg.Database.DSN(), dbConfig); err != nil {
		log.Fatalf("Failed to init database: %v", err)
	}

	// 初始化服务
	initServices(cfg)

	// 设置Gin模式
	gin.SetMode(gin.ReleaseMode)

	// 创建路由
	r := gin.Default()

	// 加载模板和静态文件 (根据构建模式自动选择嵌入或文件系统)
	funcMap := template.FuncMap{
		"safeHTML": func(s string) template.HTML { return template.HTML(s) },
	}
	if err := web.LoadTemplates(r, funcMap); err != nil {
		log.Fatalf("Failed to load templates: %v", err)
	}
	if err := web.SetupStatic(r, cfg.Storage.DataDir); err != nil {
		log.Fatalf("Failed to setup static files: %v", err)
	}

	// 打印运行模式
	if web.IsEmbedded() {
		log.Println("Running in RELEASE mode (embedded resources)")
	} else {
		log.Println("Running in DEV mode (filesystem resources)")
	}

	// 注册路由
	registerRoutes(r, cfg)

	// 启动后台服务
	startBackgroundServices(cfg)

	// 启动服务器
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	log.Printf("EzPay server starting on %s", addr)

	// 优雅关闭
	go func() {
		if err := r.Run(addr); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// 等待中断信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")
	service.GetBlockchainService().Stop()
	log.Println("Server exited")
}

// initServices 初始化服务
func initServices(cfg *config.Config) {
	// 初始化安全配置
	middleware.InitRateLimiters(
		cfg.Security.RateLimitAPI,
		cfg.Security.RateLimitAPIBurst,
		cfg.Security.RateLimitLogin,
		cfg.Security.RateLimitBurst,
	)
	middleware.SetIPBlacklistCacheTTL(cfg.Security.IPBlacklistCacheTTL)

	// 初始化区块链服务（使用配置的钱包缓存TTL）
	service.GetBlockchainService().Init(cfg)
	service.GetBlockchainService().SetWalletCacheTTL(cfg.Order.WalletCacheTTL)

	// 初始化汇率服务
	rateService := service.GetRateService()
	rateService.SetCacheSeconds(cfg.Rate.CacheSeconds)
}

// registerRoutes 注册路由
func registerRoutes(r *gin.Engine, cfg *config.Config) {
	// CORS（使用配置的域名白名单）
	r.Use(middleware.CORSWithConfig(cfg.Security.CORSAllowOrigins))

	// 创建处理器
	epayHandler := handler.NewEpayHandler()
	vmqHandler := handler.NewVmqHandler()
	adminHandler := handler.NewAdminHandler(cfg)
	merchantHandler := handler.NewMerchantHandler(cfg)
	cashierHandler := handler.NewCashierHandler()
	channelHandler := handler.NewChannelHandler()

	// ============ 彩虹易支付兼容接口 ============
	// 应用API日志中间件和IP黑名单检查到支付API
	paymentAPI := r.Group("")
	paymentAPI.Use(middleware.IPBlacklistCheck())
	paymentAPI.Use(middleware.APILogger())
	{
		// 发起支付 (表单跳转)
		paymentAPI.Any("/submit.php", epayHandler.Submit)
		paymentAPI.Any("/api/submit", epayHandler.Submit)

		// API方式发起支付
		paymentAPI.Any("/mapi.php", epayHandler.MAPISubmit)
		paymentAPI.Any("/api/mapi", epayHandler.MAPISubmit)

		// 通用API (查询订单等)
		paymentAPI.GET("/api.php", epayHandler.API)
		paymentAPI.GET("/api/query", epayHandler.API)

		// 检查订单状态 (轮询)
		paymentAPI.GET("/api/check_order", epayHandler.CheckOrder)

		// ============ V免签兼容接口 ============
		paymentAPI.GET("/createOrder", vmqHandler.CreateOrder)
		paymentAPI.GET("/appHeart", vmqHandler.AppHeart)
		paymentAPI.GET("/appPush", vmqHandler.AppPush)
		paymentAPI.GET("/getState", vmqHandler.GetState)
		paymentAPI.GET("/closeOrder", vmqHandler.CloseOrder)
	}

	// ============ 收银台 ============
	r.GET("/cashier/:trade_no", cashierHandler.ShowCashier)
	r.GET("/api/order/:trade_no", cashierHandler.GetOrderInfo)

	// ============ 上游通道回调 ============
	r.GET("/channel/notify/vmq", channelHandler.VmqNotify)
	r.GET("/channel/notify/epay", channelHandler.EpayNotify)

	// ============ 管理后台 ============
	// 管理后台页面
	r.GET("/admin", func(c *gin.Context) {
		c.HTML(http.StatusOK, "admin.html", nil)
	})

	// 登录 (无需认证)
	r.POST("/admin/api/login", adminHandler.Login)

	// 需要认证的管理API
	adminAPI := r.Group("/admin/api")
	adminAPI.Use(middleware.AdminAuth(cfg))
	{
		// 仪表盘
		adminAPI.GET("/dashboard", adminHandler.Dashboard)
		adminAPI.GET("/dashboard/trend", adminHandler.DashboardTrend)
		adminAPI.GET("/dashboard/top", adminHandler.DashboardTop)

		// 订单管理
		adminAPI.GET("/orders", adminHandler.ListOrders)
		adminAPI.GET("/orders/export", adminHandler.ExportOrders)
		adminAPI.GET("/orders/:trade_no", adminHandler.GetOrder)
		adminAPI.POST("/orders/:trade_no/paid", adminHandler.MarkOrderPaid)
		adminAPI.POST("/orders/:trade_no/notify", adminHandler.RetryNotify)
		adminAPI.POST("/orders/test", adminHandler.CreateTestOrder)
		adminAPI.POST("/orders/clean", adminHandler.CleanInvalidOrders)

		// 商户管理
		adminAPI.GET("/merchants", adminHandler.ListMerchants)
		adminAPI.POST("/merchants", adminHandler.CreateMerchant)
		adminAPI.PUT("/merchants/:id", adminHandler.UpdateMerchant)
		adminAPI.GET("/merchants/:id/key", adminHandler.GetMerchantKey)
		adminAPI.POST("/merchants/:id/reset-key", adminHandler.ResetMerchantKey)
		adminAPI.POST("/merchants/:id/balance", adminHandler.AdjustMerchantBalance)

		// 钱包管理
		adminAPI.GET("/wallets", adminHandler.ListWallets)
		adminAPI.POST("/wallets", adminHandler.CreateWallet)
		adminAPI.PUT("/wallets/:id", adminHandler.UpdateWallet)
		adminAPI.DELETE("/wallets/:id", adminHandler.DeleteWallet)
		adminAPI.POST("/upload/qrcode", adminHandler.UploadQRCode)

		// 系统配置
		adminAPI.GET("/configs", adminHandler.GetConfigs)
		adminAPI.POST("/configs", adminHandler.UpdateConfigs)

		// 汇率
		adminAPI.GET("/rate", adminHandler.GetRate)
		adminAPI.POST("/rate/refresh", adminHandler.RefreshRate)

		// 交易日志
		adminAPI.GET("/transactions", adminHandler.GetTransactionLogs)

		// API调用日志
		adminAPI.GET("/api-logs", adminHandler.GetAPILogs)
		adminAPI.POST("/api-logs/clean", adminHandler.CleanAPILogs)

		// IP黑名单管理
		adminAPI.GET("/ip-blacklist", adminHandler.ListIPBlacklist)
		adminAPI.POST("/ip-blacklist", adminHandler.AddIPBlacklist)
		adminAPI.DELETE("/ip-blacklist/:id", adminHandler.RemoveIPBlacklist)
		adminAPI.POST("/ip-blacklist/block", adminHandler.BlockIPFromAPILog)

		// 修改密码
		adminAPI.POST("/password", adminHandler.ChangePassword)

		// 测试机器人通知
		adminAPI.POST("/test-bot", func(c *gin.Context) {
			if err := service.GetBotService().SendTestMessage(); err != nil {
				c.JSON(http.StatusOK, gin.H{"code": -1, "msg": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "已发送测试消息"})
		})

		// 测试Telegram Bot连接
		adminAPI.POST("/telegram/test", adminHandler.TestTelegramBot)

		// 链监控管理
		adminAPI.GET("/chains", adminHandler.GetChainStatus)
		adminAPI.POST("/chains/:chain/enable", adminHandler.EnableChain)
		adminAPI.POST("/chains/:chain/disable", adminHandler.DisableChain)
		adminAPI.POST("/chains/batch", adminHandler.BatchUpdateChains)

		// 提现管理
		adminAPI.GET("/withdrawals", adminHandler.ListWithdrawals)
		adminAPI.POST("/withdrawals/:id/approve", adminHandler.ApproveWithdrawal)
		adminAPI.POST("/withdrawals/:id/reject", adminHandler.RejectWithdrawal)
		adminAPI.POST("/withdrawals/:id/complete", adminHandler.CompleteWithdrawal)

		// 提现地址审核
		adminAPI.GET("/withdraw-addresses", adminHandler.ListWithdrawAddresses)
		adminAPI.POST("/withdraw-addresses/:id/approve", adminHandler.ApproveWithdrawAddress)
		adminAPI.POST("/withdraw-addresses/:id/reject", adminHandler.RejectWithdrawAddress)

		// APP版本管理
		adminAPI.GET("/app-versions", adminHandler.ListAppVersions)
		adminAPI.POST("/app-versions", adminHandler.UploadAppVersion)
		adminAPI.PUT("/app-versions/:id", adminHandler.UpdateAppVersion)
		adminAPI.DELETE("/app-versions/:id", adminHandler.DeleteAppVersion)
	}

	// ============ 商户后台 ============
	// 商户后台页面
	r.GET("/merchant", func(c *gin.Context) {
		c.HTML(http.StatusOK, "merchant.html", nil)
	})

	// 商户登录 (无需认证)
	r.POST("/merchant/api/login", merchantHandler.Login)

	// 需要认证的商户API
	merchantAPI := r.Group("/merchant/api")
	merchantAPI.Use(middleware.MerchantAuth(cfg))
	{
		// 仪表盘
		merchantAPI.GET("/dashboard", merchantHandler.Dashboard)
		merchantAPI.GET("/dashboard/trend", merchantHandler.DashboardTrend)

		// 个人信息
		merchantAPI.GET("/profile", merchantHandler.GetProfile)
		merchantAPI.PUT("/profile", merchantHandler.UpdateProfile)
		merchantAPI.POST("/password", merchantHandler.ChangePassword)

		// API密钥
		merchantAPI.GET("/key", merchantHandler.GetKey)
		merchantAPI.POST("/key/reset", merchantHandler.ResetKey)

		// 订单管理
		merchantAPI.GET("/orders", merchantHandler.ListOrders)
		merchantAPI.GET("/orders/:trade_no", merchantHandler.GetOrder)
		merchantAPI.POST("/orders/:trade_no/confirm", merchantHandler.ConfirmPayment)
		merchantAPI.POST("/orders/:trade_no/cancel", merchantHandler.CancelOrder)
		merchantAPI.POST("/orders/test", merchantHandler.CreateTestOrder)

		// 钱包管理
		merchantAPI.GET("/wallets", merchantHandler.ListWallets)
		merchantAPI.POST("/wallets", merchantHandler.CreateWallet)
		merchantAPI.PUT("/wallets/:id", merchantHandler.UpdateWallet)
		merchantAPI.DELETE("/wallets/:id", merchantHandler.DeleteWallet)
		merchantAPI.POST("/upload/qrcode", merchantHandler.UploadQRCode)

		// 链状态 (只读)
		merchantAPI.GET("/chains", merchantHandler.GetChainStatus)

		// 提现管理
		merchantAPI.GET("/balance", merchantHandler.GetBalance)
		merchantAPI.GET("/recharge-addresses", merchantHandler.GetRechargeAddresses)
		merchantAPI.GET("/withdrawals", merchantHandler.ListWithdrawals)
		merchantAPI.POST("/withdrawals", merchantHandler.CreateWithdrawal)

		// 提现地址管理
		merchantAPI.GET("/withdraw-addresses", merchantHandler.ListWithdrawAddresses)
		merchantAPI.POST("/withdraw-addresses", merchantHandler.CreateWithdrawAddress)
		merchantAPI.PUT("/withdraw-addresses/:id", merchantHandler.UpdateWithdrawAddress)
		merchantAPI.DELETE("/withdraw-addresses/:id", merchantHandler.DeleteWithdrawAddress)

		// 钱包模式设置
		merchantAPI.GET("/wallet-mode", merchantHandler.GetWalletMode)
		merchantAPI.PUT("/wallet-mode", merchantHandler.UpdateWalletMode)

		// Telegram Bot 信息
		merchantAPI.GET("/telegram-bot", merchantHandler.GetTelegramBotInfo)

		// 通知设置
		merchantAPI.GET("/notify-settings", merchantHandler.GetNotifySettings)
		merchantAPI.PUT("/notify-settings", merchantHandler.UpdateNotifySettings)

		// 监控客户端配置
		merchantAPI.GET("/monitor-config", merchantHandler.GetMonitorConfig)
	}

	// 健康检查 - 简单版本（用于负载均衡器）
	r.GET("/health", func(c *gin.Context) {
		// 快速检查数据库连接
		if err := model.CheckDBHealth(); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "unhealthy",
				"error":  "database connection failed",
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
		})
	})

	// 健康检查 - 详细版本（用于监控系统）
	r.GET("/health/detail", func(c *gin.Context) {
		health := gin.H{
			"status":    "ok",
			"timestamp": time.Now().Format(time.RFC3339),
			"version":   "1.0.0",
		}

		// 检查数据库
		dbStatus := "ok"
		if err := model.CheckDBHealth(); err != nil {
			dbStatus = "error: " + err.Error()
			health["status"] = "degraded"
		}
		health["database"] = gin.H{
			"status": dbStatus,
			"stats":  model.GetDBStats(),
		}

		// 检查区块链服务
		blockchainStatus := service.GetBlockchainService().GetListenerStatus()
		enabledChains := 0
		runningChains := 0
		for _, status := range blockchainStatus {
			if s, ok := status.(map[string]interface{}); ok {
				if enabled, _ := s["enabled"].(bool); enabled {
					enabledChains++
				}
				if running, _ := s["running"].(bool); running {
					runningChains++
				}
			}
		}
		health["blockchain"] = gin.H{
			"enabled_chains": enabledChains,
			"running_chains": runningChains,
		}

		// 返回状态码
		statusCode := http.StatusOK
		if health["status"] == "degraded" {
			statusCode = http.StatusOK // 降级但仍可用
		} else if health["status"] == "unhealthy" {
			statusCode = http.StatusServiceUnavailable
		}

		c.JSON(statusCode, health)
	})

	// ============ APP公开接口 ============
	r.GET("/api/app/version", adminHandler.GetLatestAppVersion)  // APP版本检测
	r.GET("/api/app/download", adminHandler.DownloadApp)         // APP下载

	// ============ 公开支付接口 ============
	r.GET("/api/payment-types", epayHandler.GetPaymentTypes)     // 获取支持的支付类型
}

// startBackgroundServices 启动后台服务
func startBackgroundServices(cfg *config.Config) {
	// 启动区块链监控
	service.GetBlockchainService().Start()

	// 启动订单过期处理
	service.GetOrderService().StartExpireWorker()

	// 启动通知重试
	service.GetNotifyService().StartNotifyWorker()

	// 启动每日报告
	service.GetBotService().StartDailyReportWorker()

	// 启动Telegram通知服务 - 先从数据库加载配置
	var telegramCfg model.SystemConfig
	if err := model.GetDB().Where("`key` = ?", "telegram_bot_token").First(&telegramCfg).Error; err == nil && telegramCfg.Value != "" {
		service.GetTelegramService().UpdateConfig(true, telegramCfg.Value)
	}
	service.GetTelegramService().Start()

	log.Println("Background services started")
}
