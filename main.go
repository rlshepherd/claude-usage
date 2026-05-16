// claude-usage: show Claude Code subscription usage + extra-usage budget.
//
// Reads the OAuth access token from the macOS Keychain and calls
// https://api.anthropic.com/api/oauth/usage.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const (
	keychainService = "Claude Code-credentials"
	usageURL        = "https://api.anthropic.com/api/oauth/usage"
	userAgent       = "claude-code/2.0.31"
	anthropicBeta   = "oauth-2025-04-20"
	httpTimeout     = 15 * time.Second
)

type Credentials struct {
	ClaudeAIOAuth struct {
		AccessToken      string   `json:"accessToken"`
		RefreshToken     string   `json:"refreshToken"`
		ExpiresAt        int64    `json:"expiresAt"`
		Scopes           []string `json:"scopes"`
		SubscriptionType string   `json:"subscriptionType"`
	} `json:"claudeAiOauth"`
}

type Window struct {
	Utilization float64 `json:"utilization"`
	ResetsAt    *string `json:"resets_at"`
}

type ExtraUsage struct {
	IsEnabled      bool     `json:"is_enabled"`
	MonthlyLimit   float64  `json:"monthly_limit"`
	UsedCredits    float64  `json:"used_credits"`
	Utilization    float64  `json:"utilization"`
	Currency       string   `json:"currency"`
	DisabledReason *string  `json:"disabled_reason"`
}

type UsageResponse struct {
	FiveHour          *Window     `json:"five_hour"`
	SevenDay          *Window     `json:"seven_day"`
	SevenDayOAuthApps *Window     `json:"seven_day_oauth_apps"`
	SevenDayOpus      *Window     `json:"seven_day_opus"`
	SevenDaySonnet    *Window     `json:"seven_day_sonnet"`
	SevenDayCowork    *Window     `json:"seven_day_cowork"`
	SevenDayOmelette  *Window     `json:"seven_day_omelette"`
	ExtraUsage        *ExtraUsage `json:"extra_usage"`
}

var (
	errNoCredentials = errors.New("no Claude Code credentials in Keychain (try `claude login`)")
	errUnauthorized  = errors.New("token rejected (expired? try `claude login` again)")
)

func getAccessToken() (string, error) {
	if runtime.GOOS != "darwin" {
		return "", fmt.Errorf("Keychain lookup is macOS-only; got GOOS=%s", runtime.GOOS)
	}
	out, err := exec.Command("security", "find-generic-password", "-s", keychainService, "-w").Output()
	if err != nil {
		return "", errNoCredentials
	}
	var creds Credentials
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(out))), &creds); err != nil {
		return "", fmt.Errorf("parse Keychain credentials: %w", err)
	}
	if creds.ClaudeAIOAuth.AccessToken == "" {
		return "", errors.New("credentials present but accessToken is empty")
	}
	return creds.ClaudeAIOAuth.AccessToken, nil
}

func fetchRaw(token string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, usageURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("anthropic-beta", anthropicBeta)

	resp, err := (&http.Client{Timeout: httpTimeout}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return nil, errUnauthorized
	case resp.StatusCode >= 400:
		return nil, fmt.Errorf("api %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}

func formatReset(ts *string) string {
	if ts == nil || *ts == "" {
		return "unknown"
	}
	t, err := time.Parse(time.RFC3339Nano, *ts)
	if err != nil {
		if t, err = time.Parse(time.RFC3339, *ts); err != nil {
			return *ts
		}
	}
	d := time.Until(t).Round(time.Minute)
	if d <= 0 {
		return "now"
	}
	if d < time.Hour {
		return fmt.Sprintf("in %dm", int(d.Minutes()))
	}
	h := int(d.Hours())
	m := int(d.Minutes()) - h*60
	if m == 0 {
		return fmt.Sprintf("in %dh", h)
	}
	return fmt.Sprintf("in %dh%dm", h, m)
}

func renderWindow(label string, w *Window) string {
	if w == nil {
		return ""
	}
	remaining := 100 - w.Utilization
	if remaining < 0 {
		remaining = 0
	}
	return fmt.Sprintf("%-22s %5.1f%% used  (%.1f%% left, resets %s)",
		label, w.Utilization, remaining, formatReset(w.ResetsAt))
}

func printWindow(label string, w *Window) {
	if line := renderWindow(label, w); line != "" {
		fmt.Println(line)
	}
}

func run() error {
	asJSON := flag.Bool("json", false, "parsed JSON output")
	raw := flag.Bool("raw", false, "dump raw server response")
	flag.Parse()

	token, err := getAccessToken()
	if err != nil {
		return err
	}
	body, err := fetchRaw(token)
	if err != nil {
		return err
	}

	if *raw {
		var v any
		if err := json.Unmarshal(body, &v); err == nil {
			out, _ := json.MarshalIndent(v, "", "  ")
			fmt.Println(string(out))
		} else {
			fmt.Println(string(body))
		}
		return nil
	}

	var u UsageResponse
	if err := json.Unmarshal(body, &u); err != nil {
		return fmt.Errorf("decode: %w", err)
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(u)
	}

	printWindow("5-hour:", u.FiveHour)
	printWindow("7-day:", u.SevenDay)
	printWindow("7-day (opus):", u.SevenDayOpus)
	printWindow("7-day (sonnet):", u.SevenDaySonnet)
	printWindow("7-day (cowork):", u.SevenDayCowork)
	printWindow("7-day (oauth apps):", u.SevenDayOAuthApps)
	printWindow("7-day (omelette):", u.SevenDayOmelette)

	if eu := u.ExtraUsage; eu != nil {
		fmt.Println()
		if !eu.IsEnabled {
			reason := ""
			if eu.DisabledReason != nil {
				reason = " (" + *eu.DisabledReason + ")"
			}
			fmt.Printf("extra usage: disabled%s\n", reason)
		} else {
			// API returns amounts in minor units (cents); convert to whole currency.
			used := eu.UsedCredits / 100
			limit := eu.MonthlyLimit / 100
			remaining := limit - used
			if remaining < 0 {
				remaining = 0
			}
			cur := eu.Currency
			if cur == "" {
				cur = "USD"
			}
			fmt.Printf("extra usage: $%.2f / $%.2f %s used  ($%.2f remaining, %.1f%% of monthly limit)\n",
				used, limit, cur, remaining, eu.Utilization)
		}
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
