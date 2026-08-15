package main

import "testing"

func TestCalculateReimbursement(t *testing.T) {
	limit := int64(5_000_000)
	tiers := []RateTier{
		{UpToMilliKM: &limit, CentsPerKM: 73},
		{CentsPerKM: 67},
	}

	tests := []struct {
		name      string
		kmMilli   int64
		wantCents int64
	}{
		{"under first tier", 100_000, 7_300},
		{"exactly first tier", 5_000_000, 365_000},
		{"crosses tier", 6_000_000, 432_000},
		{"fractional km", 1_500, 110},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := calculateReimbursement(tc.kmMilli, tiers)
			if got != tc.wantCents {
				t.Fatalf("got %d cents, want %d", got, tc.wantCents)
			}
		})
	}
}

func TestParseKilometersMilli(t *testing.T) {
	tests := map[string]int64{
		"1":       1000,
		"1.2":     1200,
		"1.23":    1230,
		"1.234":   1234,
		"12345.6": 12345600,
	}
	for input, want := range tests {
		got, err := parseKilometersMilli(input)
		if err != nil {
			t.Fatalf("%q: %v", input, err)
		}
		if got != want {
			t.Fatalf("%q: got %d, want %d", input, got, want)
		}
	}
}
