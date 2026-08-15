package main

import "fmt"

type RateBreakdown struct {
	KilometersMilli int64
	CentsPerKM      int64
	AmountCents     int64
}

func calculateReimbursement(totalMilliKM int64, tiers []RateTier) (int64, []RateBreakdown) {
	if totalMilliKM <= 0 {
		return 0, nil
	}

	remaining := totalMilliKM
	previousLimit := int64(0)
	totalCents := int64(0)
	breakdown := make([]RateBreakdown, 0, len(tiers))

	for _, tier := range tiers {
		if remaining <= 0 {
			break
		}
		eligible := remaining
		if tier.UpToMilliKM != nil {
			capacity := *tier.UpToMilliKM - previousLimit
			if eligible > capacity {
				eligible = capacity
			}
		}
		amount := roundPositive(eligible*tier.CentsPerKM, 1000)
		breakdown = append(breakdown, RateBreakdown{
			KilometersMilli: eligible,
			CentsPerKM:      tier.CentsPerKM,
			AmountCents:     amount,
		})
		totalCents += amount
		remaining -= eligible
		if tier.UpToMilliKM != nil {
			previousLimit = *tier.UpToMilliKM
		}
	}
	return totalCents, breakdown
}

func roundPositive(numerator, denominator int64) int64 {
	return (numerator + denominator/2) / denominator
}

func formatMoney(cents int64) string {
	return fmt.Sprintf("$%d.%02d", cents/100, cents%100)
}

func formatKM(milli int64) string {
	whole := milli / 1000
	frac := milli % 1000
	if frac == 0 {
		return fmt.Sprintf("%d", whole)
	}
	if frac%100 == 0 {
		return fmt.Sprintf("%d.%d", whole, frac/100)
	}
	if frac%10 == 0 {
		return fmt.Sprintf("%d.%02d", whole, frac/10)
	}
	return fmt.Sprintf("%d.%03d", whole, frac)
}
