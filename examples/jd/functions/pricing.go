//go:build ignore

package main

import (
	"fmt"
	"time"
)

type Line struct {
	PriceMinor int64 `json:"price_minor"`
	Qty        int64 `json:"qty"`
}

type QuoteInput struct {
	Items       []Line `json:"items"`
	CouponKind  string `json:"coupon_kind"`
	CouponValue int64  `json:"coupon_value"`
	MinMinor    int64  `json:"min_minor"`
	PlusMember  bool   `json:"plus_member"`
}

type QuoteResult struct {
	TotalMinor    int64  `json:"total_minor"`
	DiscountMinor int64  `json:"discount_minor"`
	FreightMinor  int64  `json:"freight_minor"`
	PayMinor      int64  `json:"pay_minor"`
	CouponApplied bool   `json:"coupon_applied"`
	Error         string `json:"error,omitempty"`
}

// 满 99 包邮，否则 6 元运费；PLUS 会员免运费。
const freightMinor int64 = 600
const freeShipMinor int64 = 9900

func Quote(in QuoteInput) QuoteResult {
	var r QuoteResult
	for _, line := range in.Items {
		if line.Qty <= 0 {
			r.Error = "qty must be > 0"
			return r
		}
		if line.PriceMinor < 0 {
			r.Error = "price must be >= 0"
			return r
		}
		r.TotalMinor += line.PriceMinor * line.Qty
	}

	if in.CouponValue > 0 && r.TotalMinor >= in.MinMinor {
		switch in.CouponKind {
		case "amount":
			r.DiscountMinor = in.CouponValue
		case "percent":
			// 折扣券按百分比减价，value 为 1-99 的折扣力度。
			if in.CouponValue >= 100 {
				r.DiscountMinor = r.TotalMinor
			} else {
				r.DiscountMinor = r.TotalMinor * in.CouponValue / 100
			}
		default:
			r.CouponApplied = false
		}
		if r.DiscountMinor > r.TotalMinor {
			r.DiscountMinor = r.TotalMinor
		}
		r.CouponApplied = r.DiscountMinor > 0
	}

	r.PayMinor = r.TotalMinor - r.DiscountMinor
	if r.PayMinor > 0 && r.PayMinor < freeShipMinor && !in.PlusMember {
		r.FreightMinor = freightMinor
	}
	r.PayMinor += r.FreightMinor
	return r
}

type TickInput struct {
	Day string `json:"day"`
}

type DayWindow struct {
	Day   string `json:"day"`
	Label string `json:"label"`
}

// Tick 返回结算窗口：默认统计 UTC 前一天。
func Tick(in TickInput) DayWindow {
	day := in.Day
	if day == "" {
		day = time.Now().UTC().Add(-24 * time.Hour).Format("2006-01-02")
	}
	return DayWindow{Day: day, Label: fmt.Sprintf("统计 %s 的订单与 GMV", day)}
}