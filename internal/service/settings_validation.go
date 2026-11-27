package service

import (
	"fmt"
	"github.com/shopspring/decimal"
	"strconv"
	"strings"
)

func ValidateOperationalSettings(values map[string]string) error {
	for key, bounds := range map[string][2]int{"order_expire": {1, 1440}, "notify_retry": {1, 10}} {
		if values[key] == "" {
			continue
		}
		v, err := strconv.Atoi(values[key])
		if err != nil || v < bounds[0] || v > bounds[1] {
			return fmt.Errorf("%s 必须在 %d 到 %d 之间", key, bounds[0], bounds[1])
		}
	}
	for _, key := range []string{"system_wallet_fee_rate", "personal_wallet_fee_rate"} {
		if values[key] == "" {
			continue
		}
		if len(values[key]) > 20 || strings.ContainsAny(values[key], "eE") {
			return fmt.Errorf("%s 无效", key)
		}
		v, err := decimal.NewFromString(values[key])
		if err != nil || v.IsNegative() || v.GreaterThan(decimal.NewFromInt(1)) {
			return fmt.Errorf("%s 必须在 0 到 1 之间", key)
		}
	}
	return nil
}
