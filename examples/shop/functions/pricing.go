//go:build ignore

package main

import "time"

type Line struct { PriceMinor int64 `json:"price_minor"`; Qty int64 `json:"qty"` }
type QuoteInput struct { Items []Line `json:"items"`; Coupon string `json:"coupon"` }
type QuoteResult struct { TotalMinor int64 `json:"total_minor"`; DiscountMinor int64 `json:"discount_minor"` }
type TickInput struct{}
type DayWindow struct { Day string `json:"day"` }

func Quote(in QuoteInput) QuoteResult {
	var result QuoteResult
	for _, line := range in.Items { result.TotalMinor += line.PriceMinor * line.Qty }
	if in.Coupon == "DEMO10" { result.DiscountMinor = result.TotalMinor / 10; result.TotalMinor -= result.DiscountMinor }
	return result
}
func Tick(_ TickInput) DayWindow { return DayWindow{Day: time.Now().UTC().Add(-24 * time.Hour).Format("2006-01-02")} }
