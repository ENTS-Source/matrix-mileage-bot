package main

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

func parseKilometersMilli(input string) (int64, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return 0, fmt.Errorf("missing number")
	}
	if strings.HasPrefix(s, "+") {
		s = s[1:]
	}
	if strings.HasPrefix(s, "-") {
		return 0, fmt.Errorf("value must be positive")
	}
	parts := strings.Split(s, ".")
	if len(parts) > 2 || parts[0] == "" {
		return 0, fmt.Errorf("invalid number %q", input)
	}
	for _, r := range parts[0] {
		if !unicode.IsDigit(r) || r > unicode.MaxASCII {
			return 0, fmt.Errorf("invalid number %q", input)
		}
	}
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number %q", input)
	}
	frac := int64(0)
	if len(parts) == 2 {
		if len(parts[1]) == 0 || len(parts[1]) > 3 {
			return 0, fmt.Errorf("use at most 3 decimal places")
		}
		for _, r := range parts[1] {
			if r < '0' || r > '9' {
				return 0, fmt.Errorf("invalid number %q", input)
			}
		}
		padded := parts[1] + strings.Repeat("0", 3-len(parts[1]))
		frac, err = strconv.ParseInt(padded, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid number %q", input)
		}
	}
	if whole > (1<<63-1-frac)/1000 {
		return 0, fmt.Errorf("number is too large")
	}
	value := whole*1000 + frac
	if value <= 0 {
		return 0, fmt.Errorf("value must be greater than 0")
	}
	return value, nil
}
