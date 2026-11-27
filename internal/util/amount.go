package util

import (
	"errors"
	"github.com/shopspring/decimal"
	"regexp"
)

var amountPattern = regexp.MustCompile(`^[0-9]{1,12}(\.[0-9]{1,6})?$`)

func ParseAmount(value string) (decimal.Decimal, error) {
	if len(value) > 19 || !amountPattern.MatchString(value) {
		return decimal.Zero, errors.New("金额必须为普通十进制数字，最多 12 位整数及 6 位小数")
	}
	amount, err := decimal.NewFromString(value)
	if err != nil || !amount.IsPositive() {
		return decimal.Zero, errors.New("金额必须大于零")
	}
	return amount, nil
}
