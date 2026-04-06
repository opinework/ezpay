package model

import (
	"time"

	"gorm.io/gorm"
)

// AppVersion APP版本管理
type AppVersion struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	VersionCode int            `gorm:"not null;uniqueIndex" json:"version_code"`                   // 版本号（数字，用于比较）
	VersionName string         `gorm:"size:50;not null" json:"version_name"`                       // 版本名称（如 1.0.0）
	FileName    string         `gorm:"size:255;not null" json:"file_name"`                         // 文件名
	FilePath    string         `gorm:"size:500;not null" json:"file_path"`                         // 文件存储路径
	FileSize    int64          `gorm:"not null;default:0" json:"file_size"`                        // 文件大小（字节）
	MD5         string         `gorm:"size:32" json:"md5"`                                         // 文件MD5校验
	Changelog   string         `gorm:"type:text" json:"changelog"`                                 // 更新日志
	ForceUpdate bool           `gorm:"not null;default:false" json:"force_update"`                 // 是否强制更新
	Status      int8           `gorm:"not null;default:1" json:"status"`                           // 状态：0=禁用 1=启用
	Downloads   int64          `gorm:"not null;default:0" json:"downloads"`                        // 下载次数
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName 表名
func (AppVersion) TableName() string {
	return "app_versions"
}

// GetLatestVersion 获取最新启用的版本
func GetLatestAppVersion() (*AppVersion, error) {
	var version AppVersion
	err := DB.Where("status = 1").Order("version_code DESC").First(&version).Error
	if err != nil {
		return nil, err
	}
	return &version, nil
}

// GetAppVersionByCode 根据版本号获取版本
func GetAppVersionByCode(code int) (*AppVersion, error) {
	var version AppVersion
	err := DB.Where("version_code = ? AND status = 1", code).First(&version).Error
	if err != nil {
		return nil, err
	}
	return &version, nil
}

// IncrementDownloads 增加下载计数
func (v *AppVersion) IncrementDownloads() error {
	return DB.Model(v).UpdateColumn("downloads", gorm.Expr("downloads + ?", 1)).Error
}
