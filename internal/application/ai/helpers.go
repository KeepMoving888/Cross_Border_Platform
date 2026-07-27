package ai

import (
	"fmt"

	"github.com/shopspring/decimal"
)

var decimalZero = decimal.Zero

func decimalFromString(s string) (decimal.Decimal, error) {
	return decimal.NewFromString(s)
}

func decimalFromInt(i int64) decimal.Decimal {
	return decimal.NewFromInt(i)
}

// 确保 fmt 包被使用(避免 unused import 警告)
var _ = fmt.Sprintf
