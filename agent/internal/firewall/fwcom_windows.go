//go:build windows

package firewall

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	ole "github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

// Windows Firewall through its COM API (HNetCfg.FwPolicy2), the same one installers use:
// no child processes, and rules are matched by their exact program path.

const (
	fwDirIn       = 1
	fwActionAllow = 1
	fwProfileAll  = 0x7FFFFFFF
	maxRemove     = 20 // a sane upper bound for rules that can belong to one exe
)

type fwRule struct {
	Name string
	App  string
}

// withRules runs f against INetFwPolicy2.Rules on a COM-initialized locked thread.
func withRules(f func(rules *ole.IDispatch) error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
		var oe *ole.OleError
		if !errors.As(err, &oe) || oe.Code() != 1 { // S_FALSE: already initialized
			return err
		}
	}
	defer ole.CoUninitialize()

	unk, err := oleutil.CreateObject("HNetCfg.FwPolicy2")
	if err != nil {
		return fmt.Errorf("FwPolicy2: %w", err)
	}
	defer unk.Release()
	policy, err := unk.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return err
	}
	defer policy.Release()
	v, err := oleutil.GetProperty(policy, "Rules")
	if err != nil {
		return err
	}
	defer v.Clear()
	return f(v.ToIDispatch())
}

func listRules(rules *ole.IDispatch) ([]fwRule, error) {
	var out []fwRule
	err := oleutil.ForEach(rules, func(v *ole.VARIANT) error {
		defer v.Clear()
		r := v.ToIDispatch()
		out = append(out, fwRule{Name: strProp(r, "Name"), App: strProp(r, "ApplicationName")})
		return nil
	})
	return out, err
}

func strProp(d *ole.IDispatch, name string) string {
	v, err := oleutil.GetProperty(d, name)
	if err != nil {
		return ""
	}
	defer v.Clear()
	if s, ok := v.Value().(string); ok {
		return s
	}
	return ""
}

// rulesToRemove picks the rules bound to exe. Removal works by name, so a name that is
// also used by any rule of another program is skipped rather than risk touching it.
func rulesToRemove(all []fwRule, exe string) (remove map[string]int, skipped []string) {
	remove = map[string]int{}
	foreign := map[string]bool{}
	for _, r := range all {
		if strings.EqualFold(r.App, exe) {
			remove[r.Name]++
		} else {
			foreign[r.Name] = true
		}
	}
	for name := range remove {
		if foreign[name] || name == "" {
			delete(remove, name)
			skipped = append(skipped, name)
		}
	}
	return remove, skipped
}

func validExe(exe string) error {
	if !filepath.IsAbs(exe) || !strings.EqualFold(filepath.Ext(exe), ".exe") {
		return fmt.Errorf("unexpected executable path %q", exe)
	}
	return nil
}

// replaceRules removes the rules bound to exe and adds one inbound allow rule for it.
// Needs administrator rights.
func replaceRules(exe string) error {
	if err := validExe(exe); err != nil {
		return err
	}
	return withRules(func(rules *ole.IDispatch) error {
		all, err := listRules(rules)
		if err != nil {
			return fmt.Errorf("list rules: %w", err)
		}
		remove, _ := rulesToRemove(all, exe)
		total := 0
		for _, n := range remove {
			total += n
		}
		if total > maxRemove {
			return fmt.Errorf("refusing to remove %d rules", total)
		}
		for name, n := range remove {
			for i := 0; i < n; i++ {
				if _, err := oleutil.CallMethod(rules, "Remove", name); err != nil {
					break // Remove may drop all rules with that name at once
				}
			}
		}

		unk, err := oleutil.CreateObject("HNetCfg.FWRule")
		if err != nil {
			return fmt.Errorf("FWRule: %w", err)
		}
		defer unk.Release()
		rule, err := unk.QueryInterface(ole.IID_IDispatch)
		if err != nil {
			return err
		}
		defer rule.Release()
		for _, p := range []struct {
			name  string
			value any
		}{
			{"Name", "barphone"},
			{"Description", "barphone phone deck: LAN API and mDNS"},
			{"ApplicationName", exe},
			{"Direction", int32(fwDirIn)},
			{"Action", int32(fwActionAllow)},
			{"Profiles", int32(fwProfileAll)},
			{"Enabled", true},
		} {
			if _, err := oleutil.PutProperty(rule, p.name, p.value); err != nil {
				return fmt.Errorf("rule.%s: %w", p.name, err)
			}
		}
		if _, err := oleutil.CallMethod(rules, "Add", rule); err != nil {
			return fmt.Errorf("add rule: %w", err)
		}
		return nil
	})
}
