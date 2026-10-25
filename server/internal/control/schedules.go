package control

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"

	"appcontrol/server/internal/storage"
)

type Schedule struct {
	ID              string   `json:"id"`
	TimeZone        string   `json:"timeZone"`
	DaysOfWeek      []int    `json:"daysOfWeek"`
	StartMinute     int      `json:"startMinute"`
	EndMinute       int      `json:"endMinute"`
	InternetPaused  bool     `json:"internetPaused"`
	BlockedPackages []string `json:"blockedPackages"`
}

type EffectivePolicy struct {
	InternetPaused        bool
	BlockedPackages       []string
	AlwaysAllowedPackages []string
	StateKey              string
}

func normalizePackages(input []string, blocking bool) ([]string, error) {
	if len(input) > 100 {
		return nil, ErrInvalid
	}
	result := make([]string, 0, len(input))
	seen := map[string]bool{}
	for _, name := range input {
		if len(name) > 255 || !(name == "android" && !blocking || packageName.MatchString(name)) || seen[name] || blocking && protectedNamespace(name) {
			return nil, ErrInvalid
		}
		seen[name] = true
		result = append(result, name)
	}
	sort.Strings(result)
	return result, nil
}

func normalizeSchedules(input []Schedule) ([]Schedule, error) {
	if len(input) > 20 {
		return nil, ErrInvalid
	}
	result := make([]Schedule, 0, len(input))
	seen := map[string]bool{}
	for _, rule := range input {
		if len(rule.ID) < 1 || len(rule.ID) > 64 || seen[rule.ID] || len(rule.TimeZone) > 128 || rule.TimeZone == "" || rule.TimeZone == "Local" || len(rule.DaysOfWeek) < 1 || len(rule.DaysOfWeek) > 7 || rule.StartMinute < 0 || rule.StartMinute > 1439 || rule.EndMinute < 0 || rule.EndMinute > 1440 || rule.StartMinute == rule.EndMinute {
			return nil, ErrInvalid
		}
		for _, c := range rule.ID {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
				return nil, ErrInvalid
			}
		}
		if _, err := time.LoadLocation(rule.TimeZone); err != nil {
			return nil, ErrInvalid
		}
		seen[rule.ID] = true
		days := map[int]bool{}
		rule.DaysOfWeek = append([]int{}, rule.DaysOfWeek...)
		for _, day := range rule.DaysOfWeek {
			if day < 1 || day > 7 || days[day] {
				return nil, ErrInvalid
			}
			days[day] = true
		}
		sort.Ints(rule.DaysOfWeek)
		var err error
		rule.BlockedPackages, err = normalizePackages(rule.BlockedPackages, true)
		if err != nil {
			return nil, err
		}
		result = append(result, rule)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func ResolvePolicy(policy Policy, now time.Time) (EffectivePolicy, error) {
	blocked := map[string]bool{}
	allowed := map[string]bool{}
	for _, name := range policy.BlockedPackages {
		blocked[name] = true
	}
	for _, name := range policy.AlwaysAllowedPackages {
		allowed[name] = true
	}
	paused := policy.InternetPaused
	for _, rule := range policy.Schedules {
		zone, err := time.LoadLocation(rule.TimeZone)
		if err != nil {
			return EffectivePolicy{}, err
		}
		local := now.In(zone)
		day := (int(local.Weekday())+6)%7 + 1
		minute := local.Hour()*60 + local.Minute()
		matches := func(d int) bool {
			for _, value := range rule.DaysOfWeek {
				if d == value {
					return true
				}
			}
			return false
		}
		active := false
		if rule.StartMinute < rule.EndMinute {
			active = matches(day) && minute >= rule.StartMinute && minute < rule.EndMinute
		} else {
			active = minute >= rule.StartMinute && matches(day) || minute < rule.EndMinute && matches((day+5)%7+1)
		}
		if active {
			paused = paused || rule.InternetPaused
			for _, name := range rule.BlockedPackages {
				blocked[name] = true
			}
		}
	}
	result := EffectivePolicy{InternetPaused: paused, BlockedPackages: []string{}, AlwaysAllowedPackages: []string{}}
	for name := range blocked {
		if !allowed[name] {
			result.BlockedPackages = append(result.BlockedPackages, name)
		}
	}
	for name := range allowed {
		result.AlwaysAllowedPackages = append(result.AlwaysAllowedPackages, name)
	}
	sort.Strings(result.BlockedPackages)
	sort.Strings(result.AlwaysAllowedPackages)
	canonical := "app-control-effective-policy-v1\n" + strconv.FormatInt(policy.Version, 10) + "\n" + strconv.FormatBool(paused) + "\n" + strings.Join(result.BlockedPackages, "\n") + "\n--allowed--\n" + strings.Join(result.AlwaysAllowedPackages, "\n") + "\n"
	digest := sha256.Sum256([]byte(canonical))
	result.StateKey = hex.EncodeToString(digest[:])
	return result, nil
}

func (s *Service) UpdatePolicyRules(ctx context.Context, parentID, deviceID string, expected int64, paused bool, packages, allowed []string, schedules []Schedule) (Policy, error) {
	if expected < 0 || expected >= 1<<53 {
		return Policy{}, ErrInvalid
	}
	blocked, err := normalizePackages(packages, true)
	if err != nil {
		return Policy{}, err
	}
	normalizedAllowed, err := normalizePackages(allowed, false)
	if err != nil {
		return Policy{}, err
	}
	rules, err := normalizeSchedules(schedules)
	if err != nil {
		return Policy{}, err
	}
	targets := map[string]bool{}
	for _, name := range blocked {
		targets[name] = true
	}
	for _, rule := range rules {
		for _, name := range rule.BlockedPackages {
			targets[name] = true
		}
	}
	if len(targets) > 100 {
		return Policy{}, ErrInvalid
	}
	policy := Policy{FormatVersion: 1, Version: expected + 1, InternetPaused: paused, BlockedPackages: blocked, AlwaysAllowedPackages: normalizedAllowed, Schedules: rules, UpdatedAt: s.now().UTC()}
	if len(rules) > 0 || len(normalizedAllowed) > 0 {
		policy.FormatVersion = 2
	}
	encoded, err := json.Marshal(policy)
	if err != nil {
		return Policy{}, err
	}
	if len(encoded) > 64*1024 {
		return Policy{}, ErrInvalid
	}
	encodedBlocked, _ := json.Marshal(blocked)
	encodedAllowed, _ := json.Marshal(normalizedAllowed)
	encodedRules, _ := json.Marshal(rules)
	err = storage.WithinTransaction(ctx, s.db, func(tx *sql.Tx) error {
		if err := owned(ctx, tx, parentID, deviceID); err != nil {
			return err
		}
		var current int64
		err := tx.QueryRowContext(ctx, "SELECT version FROM device_policies WHERE device_id=?", deviceID).Scan(&current)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if current != expected {
			return ErrConflict
		}
		allTargets := make([]string, 0, len(targets))
		for name := range targets {
			allTargets = append(allTargets, name)
		}
		if err = validateInventoryTargets(ctx, tx, deviceID, allTargets); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO device_policies(device_id,parent_account_id,version,internet_paused,blocked_packages,updated_at,always_allowed_packages,schedules) VALUES(?,?,?,?,?,?,?,?)
   ON CONFLICT(device_id) DO UPDATE SET version=excluded.version,internet_paused=excluded.internet_paused,blocked_packages=excluded.blocked_packages,updated_at=excluded.updated_at,always_allowed_packages=excluded.always_allowed_packages,schedules=excluded.schedules`, deviceID, parentID, policy.Version, paused, string(encodedBlocked), policy.UpdatedAt.Format(time.RFC3339Nano), string(encodedAllowed), string(encodedRules))
		return err
	})
	if err == nil {
		s.notify(ctx, deviceID)
	}
	return policy, err
}

func readPolicy(ctx context.Context, tx *sql.Tx, deviceID string) (Policy, error) {
	policy := Policy{FormatVersion: 1, BlockedPackages: []string{}, AlwaysAllowedPackages: []string{}, Schedules: []Schedule{}}
	var blocked, allowed, rules, updated string
	err := tx.QueryRowContext(ctx, "SELECT version,internet_paused,blocked_packages,updated_at,always_allowed_packages,schedules FROM device_policies WHERE device_id=?", deviceID).Scan(&policy.Version, &policy.InternetPaused, &blocked, &updated, &allowed, &rules)
	if errors.Is(err, sql.ErrNoRows) {
		return policy, nil
	}
	if err != nil {
		return policy, err
	}
	for _, item := range []struct {
		raw    string
		target any
	}{{blocked, &policy.BlockedPackages}, {allowed, &policy.AlwaysAllowedPackages}, {rules, &policy.Schedules}} {
		if err = json.Unmarshal([]byte(item.raw), item.target); err != nil {
			return policy, err
		}
	}
	if len(policy.Schedules) > 0 || len(policy.AlwaysAllowedPackages) > 0 {
		policy.FormatVersion = 2
	}
	policy.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated)
	return policy, err
}
