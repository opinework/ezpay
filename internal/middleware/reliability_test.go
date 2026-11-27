package middleware

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ezpay/config"
	"ezpay/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
)

func TestReliabilityAdminTokenValidation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "auth.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })
	if err := db.AutoMigrate(&model.Admin{}); err != nil {
		t.Fatal(err)
	}
	admin := model.Admin{Username: "review-admin", Status: 1}
	db.Create(&admin)
	old := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = old })
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "review-signing-secret"}}
	r := gin.New()
	r.GET("/admin", AdminAuth(cfg), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	for _, test := range []struct {
		name      string
		method    jwt.SigningMethod
		expiry    bool
		tokenType string
		status    int
		want      int
	}{
		{"valid", jwt.SigningMethodHS256, true, "admin", 1, 204},
		{"no-expiry", jwt.SigningMethodHS256, false, "admin", 1, 401},
		{"wrong-algorithm", jwt.SigningMethodHS512, true, "admin", 1, 401},
		{"merchant-token", jwt.SigningMethodHS256, true, "merchant", 1, 401},
		{"disabled", jwt.SigningMethodHS256, true, "admin", 0, 401},
	} {
		t.Run(test.name, func(t *testing.T) {
			db.Model(&admin).Update("status", test.status)
			claims := jwt.MapClaims{"type": test.tokenType, "admin_id": admin.ID, "username": admin.Username}
			if test.expiry {
				claims["exp"] = time.Now().Add(time.Hour).Unix()
			}
			token, err := jwt.NewWithClaims(test.method, claims).SignedString([]byte(cfg.JWT.Secret))
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest("GET", "/admin", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			response := httptest.NewRecorder()
			r.ServeHTTP(response, req)
			if response.Code != test.want {
				t.Fatal(response.Code, response.Body.String())
			}
		})
	}
}

func TestReliabilitySensitiveLogMasking(t *testing.T) {
	for _, input := range []string{
		"sign=secret-one&sign=secret-two&%6bey=secret-three&name=order",
		`{"nested":{"password":"secret-one"},"items":[{"api_v3_key":"secret-two"}],"token":"secret-three"}`,
		"key=secret-one&malformed=%XX",
	} {
		masked := maskSensitiveData(input)
		if strings.Contains(masked, "secret-") {
			t.Fatal("credential leaked", masked)
		}
	}
	r := gin.New()
	r.Use(APILogger())
	r.POST("/pay", func(c *gin.Context) { t.Fatal("oversized request reached handler") })
	response := httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest("POST", "/pay", strings.NewReader(strings.Repeat("x", (1<<20)+1))))
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatal(response.Code)
	}
}

func TestReliabilityRateLimit(t *testing.T) {
	limiter := NewRateLimiter(0, 1)
	defer close(limiter.stopChan)
	r := gin.New()
	r.GET("/login", RateLimitWithConfig(limiter), func(c *gin.Context) { c.Status(204) })
	for i, want := range []int{204, 429} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest("GET", "/login", nil)
		request.RemoteAddr = "203.0.113.1:1234"
		r.ServeHTTP(response, request)
		if response.Code != want {
			t.Fatal(i, response.Code)
		}
	}
}
