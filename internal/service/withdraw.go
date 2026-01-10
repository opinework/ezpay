package service

import (
	"errors"
	"sync"
	"time"

	"ezpay/internal/model"

	"gorm.io/gorm"
)

// WithdrawService 提现服务
type WithdrawService struct{}

var (
	withdrawService     *WithdrawService
	withdrawServiceOnce sync.Once
)

// GetWithdrawService 获取提现服务实例
func GetWithdrawService() *WithdrawService {
	withdrawServiceOnce.Do(func() {
		withdrawService = &WithdrawService{}
	})
	return withdrawService
}

// CreateWithdrawal 创建提现申请
func (s *WithdrawService) CreateWithdrawal(merchantID uint, req *WithdrawRequest) (*model.Withdrawal, error) {
	// 获取商户信息
	var merchant model.Merchant
	if err := model.GetDB().First(&merchant, merchantID).Error; err != nil {
		return nil, errors.New("商户不存在")
	}

	// 检查可用余额
	availableBalance := merchant.Balance - merchant.FrozenBalance
	if req.Amount > availableBalance {
		return nil, errors.New("可用余额不足")
	}

	// 最低提现金额检查: 50 USDT
	if req.Amount < 50 {
		return nil, errors.New("最低提现金额为50 USDT")
	}

	// 固定手续费: 1 USDT
	fee := 1.0
	realAmount := req.Amount - fee

	// 验证提现地址
	if req.AddressID == 0 {
		return nil, errors.New("请选择提现地址")
	}

	var address model.WithdrawAddress
	if err := model.GetDB().Where("id = ? AND merchant_id = ?", req.AddressID, merchantID).First(&address).Error; err != nil {
		return nil, errors.New("提现地址不存在")
	}

	// 检查地址是否已审核通过
	if address.Status != model.WithdrawAddressApproved {
		return nil, errors.New("该提现地址尚未审核通过，请等待管理员审核")
	}

	// 创建提现记录
	withdrawal := &model.Withdrawal{
		MerchantID:  merchantID,
		Amount:      req.Amount,
		Fee:         fee,
		RealAmount:  realAmount,
		PayMethod:   address.Chain,
		Account:     address.Address,
		AccountName: address.Label,
		BankName:    "",
		Status:      model.WithdrawStatusPending,
		Remark:      req.Remark,
	}

	// 开启事务
	err := model.GetDB().Transaction(func(tx *gorm.DB) error {
		// 冻结余额
		if err := tx.Model(&model.Merchant{}).Where("id = ?", merchantID).
			Update("frozen_balance", gorm.Expr("frozen_balance + ?", req.Amount)).Error; err != nil {
			return err
		}

		// 创建提现记录
		if err := tx.Create(withdrawal).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// 发送Telegram通知 - 提现申请
	go GetTelegramService().NotifyWithdrawApplied(withdrawal)

	return withdrawal, nil
}

// WithdrawRequest 提现请求
type WithdrawRequest struct {
	Amount    float64 `json:"amount" binding:"required,gt=0"`
	AddressID uint    `json:"address_id" binding:"required"` // 提现地址ID
	Remark    string  `json:"remark"`
}

// ListWithdrawals 获取提现记录列表
func (s *WithdrawService) ListWithdrawals(merchantID uint, status *model.WithdrawStatus, page, pageSize int) ([]model.Withdrawal, int64, error) {
	query := model.GetDB().Model(&model.Withdrawal{})

	if merchantID > 0 {
		query = query.Where("merchant_id = ?", merchantID)
	}
	if status != nil {
		query = query.Where("status = ?", *status)
	}

	var total int64
	query.Count(&total)

	var withdrawals []model.Withdrawal
	offset := (page - 1) * pageSize
	if err := query.Preload("Merchant").Order("id DESC").Offset(offset).Limit(pageSize).Find(&withdrawals).Error; err != nil {
		return nil, 0, err
	}

	return withdrawals, total, nil
}

// GetWithdrawal 获取单个提现记录
func (s *WithdrawService) GetWithdrawal(id uint) (*model.Withdrawal, error) {
	var withdrawal model.Withdrawal
	if err := model.GetDB().Preload("Merchant").First(&withdrawal, id).Error; err != nil {
		return nil, err
	}
	return &withdrawal, nil
}

// ApproveWithdrawal 审核通过提现
func (s *WithdrawService) ApproveWithdrawal(id uint, adminRemark string) error {
	var withdrawal model.Withdrawal
	if err := model.GetDB().First(&withdrawal, id).Error; err != nil {
		return errors.New("提现记录不存在")
	}

	if withdrawal.Status != model.WithdrawStatusPending {
		return errors.New("该提现申请已处理")
	}

	now := time.Now()
	if err := model.GetDB().Model(&withdrawal).Updates(map[string]interface{}{
		"status":       model.WithdrawStatusApproved,
		"admin_remark": adminRemark,
		"processed_at": &now,
	}).Error; err != nil {
		return err
	}

	// 发送Telegram通知 - 提现审批通过
	go GetTelegramService().NotifyWithdrawApproved(&withdrawal)

	return nil
}

// RejectWithdrawal 拒绝提现
func (s *WithdrawService) RejectWithdrawal(id uint, adminRemark string) error {
	var withdrawal model.Withdrawal
	if err := model.GetDB().First(&withdrawal, id).Error; err != nil {
		return errors.New("提现记录不存在")
	}

	if withdrawal.Status != model.WithdrawStatusPending {
		return errors.New("该提现申请已处理")
	}

	now := time.Now()

	// 开启事务
	err := model.GetDB().Transaction(func(tx *gorm.DB) error {
		// 更新状态
		if err := tx.Model(&withdrawal).Updates(map[string]interface{}{
			"status":       model.WithdrawStatusRejected,
			"admin_remark": adminRemark,
			"processed_at": &now,
		}).Error; err != nil {
			return err
		}

		// 解冻余额
		if err := tx.Model(&model.Merchant{}).Where("id = ?", withdrawal.MerchantID).
			Update("frozen_balance", gorm.Expr("frozen_balance - ?", withdrawal.Amount)).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return err
	}

	// 发送Telegram通知 - 提现被拒绝
	go GetTelegramService().NotifyWithdrawRejected(&withdrawal, adminRemark)

	return nil
}

// CompleteWithdrawal 完成打款
func (s *WithdrawService) CompleteWithdrawal(id uint, adminRemark string) error {
	var withdrawal model.Withdrawal
	if err := model.GetDB().First(&withdrawal, id).Error; err != nil {
		return errors.New("提现记录不存在")
	}

	if withdrawal.Status != model.WithdrawStatusApproved {
		return errors.New("该提现申请未审核通过")
	}

	now := time.Now()

	// 开启事务
	err := model.GetDB().Transaction(func(tx *gorm.DB) error {
		// 更新状态
		if err := tx.Model(&withdrawal).Updates(map[string]interface{}{
			"status":       model.WithdrawStatusPaid,
			"admin_remark": adminRemark,
			"processed_at": &now,
		}).Error; err != nil {
			return err
		}

		// 扣除余额和冻结余额
		if err := tx.Model(&model.Merchant{}).Where("id = ?", withdrawal.MerchantID).Updates(map[string]interface{}{
			"balance":        gorm.Expr("balance - ?", withdrawal.Amount),
			"frozen_balance": gorm.Expr("frozen_balance - ?", withdrawal.Amount),
		}).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return err
	}

	// 发送Telegram通知 - 提现已打款
	go GetTelegramService().NotifyWithdrawPaid(&withdrawal)

	return nil
}

// getWithdrawFeeRate 获取提现手续费率
func (s *WithdrawService) getWithdrawFeeRate() float64 {
	var config model.SystemConfig
	if err := model.GetDB().Where("`key` = ?", "withdraw_fee_rate").First(&config).Error; err != nil {
		return 0 // 默认无手续费
	}
	var rate float64
	if err := model.GetDB().Raw("SELECT CAST(? AS DECIMAL(5,4))", config.Value).Scan(&rate).Error; err != nil {
		return 0
	}
	return rate
}

// AddMerchantBalance 增加商户余额 (订单完成时调用)
// feeType: 1=余额扣除(个人收款码), 2=收款扣除(系统收款码)
// fee: 订单手续费
// 个人收款码：收款不入余额，但从余额扣除手续费
// 系统收款码：收款入余额（扣除手续费后）
func (s *WithdrawService) AddMerchantBalance(merchantID uint, amount float64, fee float64, feeType model.FeeType) error {
	if feeType == model.FeeTypeBalance {
		// 个人收款码模式：
		// - 收款不进入余额（商户自己收到钱了）
		// - 手续费已预扣(冻结)，解冻并从余额扣除手续费
		return model.GetDB().Model(&model.Merchant{}).Where("id = ?", merchantID).Updates(map[string]interface{}{
			"balance":        gorm.Expr("balance - ?", fee),
			"frozen_balance": gorm.Expr("frozen_balance - ?", fee),
		}).Error
	}

	// 系统收款码模式：手续费从收款扣除，剩余金额入余额
	realAmount := amount - fee
	return model.GetDB().Model(&model.Merchant{}).Where("id = ?", merchantID).
		Update("balance", gorm.Expr("balance + ?", realAmount)).Error
}

// RefundPreChargedFee 退还预扣的手续费 (订单失败/取消时)
func (s *WithdrawService) RefundPreChargedFee(merchantID uint, fee float64) error {
	if fee <= 0 {
		return nil
	}
	return model.GetDB().Model(&model.Merchant{}).Where("id = ?", merchantID).
		Update("frozen_balance", gorm.Expr("frozen_balance - ?", fee)).Error
}
