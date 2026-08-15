package main

import (
	"fmt"
	"math"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Matrix          MatrixConfig        `yaml:"matrix"`
	AuthorizedUsers []string            `yaml:"authorized_users"`
	Purpose         string              `yaml:"purpose"`
	Timezone        string              `yaml:"timezone"`
	StoragePath     string              `yaml:"storage_path"`
	Reimbursement   ReimbursementConfig `yaml:"reimbursement"`
}

type MatrixConfig struct {
	Homeserver     string   `yaml:"homeserver"`
	UserID         string   `yaml:"user_id"`
	AccessToken    string   `yaml:"access_token"`
	AllowedRoomIDs []string `yaml:"allowed_room_ids"`
}

type ReimbursementConfig struct {
	Currency string         `yaml:"currency"`
	Tiers    []RateTierYAML `yaml:"tiers"`
}

type RateTierYAML struct {
	UpToKM    *float64 `yaml:"up_to_km,omitempty"`
	RatePerKM float64  `yaml:"rate_per_km"`
}

type RateTier struct {
	UpToMilliKM *int64
	CentsPerKM  int64
}

func loadConfig(path string) (*Config, []RateTier, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, nil, fmt.Errorf("parse config: %w", err)
	}

	if cfg.Purpose == "" {
		cfg.Purpose = "2026 Move"
	}
	if cfg.Timezone == "" {
		cfg.Timezone = "America/Edmonton"
	}
	if cfg.StoragePath == "" {
		cfg.StoragePath = "./mileage.db"
	}
	if cfg.Reimbursement.Currency == "" {
		cfg.Reimbursement.Currency = "CAD"
	}

	if strings.TrimSpace(cfg.Matrix.Homeserver) == "" {
		return nil, nil, fmt.Errorf("matrix.homeserver is required")
	}
	if strings.TrimSpace(cfg.Matrix.UserID) == "" {
		return nil, nil, fmt.Errorf("matrix.user_id is required")
	}
	if strings.TrimSpace(cfg.Matrix.AccessToken) == "" {
		return nil, nil, fmt.Errorf("matrix.access_token is required")
	}
	if len(cfg.AuthorizedUsers) == 0 {
		return nil, nil, fmt.Errorf("authorized_users must contain at least one Matrix user ID")
	}
	if len(cfg.Reimbursement.Tiers) == 0 {
		return nil, nil, fmt.Errorf("reimbursement.tiers must contain at least one tier")
	}

	tiers := make([]RateTier, 0, len(cfg.Reimbursement.Tiers))
	var previousLimit int64
	for i, input := range cfg.Reimbursement.Tiers {
		if input.RatePerKM <= 0 {
			return nil, nil, fmt.Errorf("reimbursement.tiers[%d].rate_per_km must be positive", i)
		}
		cents := int64(math.Round(input.RatePerKM * 100))
		if math.Abs(float64(cents)/100-input.RatePerKM) > 0.000001 {
			return nil, nil, fmt.Errorf("reimbursement.tiers[%d].rate_per_km must have at most 2 decimal places", i)
		}

		tier := RateTier{CentsPerKM: cents}
		if input.UpToKM != nil {
			if *input.UpToKM <= 0 {
				return nil, nil, fmt.Errorf("reimbursement.tiers[%d].up_to_km must be positive", i)
			}
			limit := int64(math.Round(*input.UpToKM * 1000))
			if limit <= previousLimit {
				return nil, nil, fmt.Errorf("reimbursement tier limits must be strictly increasing")
			}
			previousLimit = limit
			tier.UpToMilliKM = &limit
		} else if i != len(cfg.Reimbursement.Tiers)-1 {
			return nil, nil, fmt.Errorf("only the final reimbursement tier may omit up_to_km")
		}
		tiers = append(tiers, tier)
	}

	if tiers[len(tiers)-1].UpToMilliKM != nil {
		return nil, nil, fmt.Errorf("the final reimbursement tier must omit up_to_km so all mileage has a rate")
	}

	// Normalize these lists so duplicates in the YAML don't create surprising behavior.
	cfg.AuthorizedUsers = uniqueSorted(cfg.AuthorizedUsers)
	cfg.Matrix.AllowedRoomIDs = uniqueSorted(cfg.Matrix.AllowedRoomIDs)

	return &cfg, tiers, nil
}

func uniqueSorted(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
