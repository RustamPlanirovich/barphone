//go:build windows

package firewall

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"barphone/agent/internal/winutil"
)

func exePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

func psQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// rulesForExe sets $rules to the firewall rules bound to $exe (read-only status check).
// Filters are collected first and walked one by one, so an empty match can never fall
// through to "all rules".
const rulesForExe = `$filters = @(Get-NetFirewallApplicationFilter -Program $exe -ErrorAction SilentlyContinue |
  Where-Object { $_.Program -ieq $exe })
$rules = @(foreach ($f in $filters) { Get-NetFirewallRule -AssociatedNetFirewallApplicationFilter $f -ErrorAction SilentlyContinue })`

func check(ctx context.Context, iface string) Status {
	st := Status{Supported: true, Network: iface}
	exe, err := exePath()
	if err != nil {
		st.Error = err.Error()
		return st
	}
	script := `$exe = ` + psQuote(exe) + `
` + rulesForExe + `
$rules = @($rules | Where-Object { $_.Enabled -eq 'True' -and $_.Direction -eq 'Inbound' })
[pscustomobject]@{
  allow    = @($rules | Where-Object { $_.Action -eq 'Allow' } | ForEach-Object { [string]$_.Profile })
  block    = @($rules | Where-Object { $_.Action -eq 'Block' } | ForEach-Object { [string]$_.Profile })
  profiles = @(Get-NetConnectionProfile | ForEach-Object { [pscustomobject]@{ alias = $_.InterfaceAlias; category = [string]$_.NetworkCategory } })
} | ConvertTo-Json -Compress -Depth 4`
	out, err := winutil.PowerShell(ctx, script)
	if err != nil {
		st.Error = err.Error()
		return st
	}
	var res struct {
		Allow    []string `json:"allow"`
		Block    []string `json:"block"`
		Profiles []struct {
			Alias    string `json:"alias"`
			Category string `json:"category"`
		} `json:"profiles"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &res); err != nil {
		st.Error = err.Error()
		return st
	}
	for _, p := range res.Profiles {
		if p.Alias == iface || (iface == "" && st.Category == "") {
			st.Network, st.Category = p.Alias, p.Category
		}
	}
	profile := map[string]string{"Public": "Public", "Private": "Private", "DomainAuthenticated": "Domain"}[st.Category]
	if profile == "" {
		profile = "Public" // unknown networks are treated as public by Windows
	}
	covers := func(list []string) bool {
		for _, p := range list {
			if strings.Contains(p, "Any") || strings.Contains(p, profile) {
				return true
			}
		}
		return false
	}
	// Block rules win over allow rules in Windows Firewall.
	st.Allowed = covers(res.Allow) && !covers(res.Block)
	st.Checked = true
	return st
}

// ElevatedFlag makes barphone-agent apply the firewall rule and exit (see main).
const ElevatedFlag = "-firewall-allow"

func allow() error {
	exe, err := exePath()
	if err != nil {
		return err
	}
	// The agent re-runs itself elevated, and that process only calls ApplyAllowRule.
	// No PowerShell or other child processes: antivirus/EDR kills and quarantines
	// unsigned programs that launch hidden encoded PowerShell (an earlier version did).
	return winutil.RunElevated(exe, ElevatedFlag)
}

// ApplyAllowRule runs in the elevated process. It replaces the rules bound to this very
// executable (including the Block rules Windows creates when its own prompt is dismissed)
// with one inbound allow rule on all profiles: Windows 11 marks new home networks as
// Public, and the agent itself only serves paired phones.
func ApplyAllowRule() error {
	exe, err := exePath()
	if err != nil {
		return err
	}
	return replaceRules(exe)
}
