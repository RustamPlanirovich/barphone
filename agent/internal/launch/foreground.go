package launch

import "strings"

// ForegroundApp identifies the application in front on the PC. It deliberately holds
// identity only (keys and a product name), never window titles: titles carry document
// and site names.
type ForegroundApp struct {
	Keys []string `json:"keys"` // e.g. "exe:code.exe", "aumid:…", "class:cabinetwclass", "bundle:…"
	Name string   `json:"name"`
}

// ID is a stable identity for comparisons ("did the app change?").
func (a ForegroundApp) ID() string { return strings.Join(a.Keys, "|") }

// Shell surfaces that briefly take the foreground: switching to them must not change
// the phone's profile.
var shellExes = map[string]bool{
	"searchhost.exe": true, "searchapp.exe": true, "searchui.exe": true,
	"startmenuexperiencehost.exe": true, "shellexperiencehost.exe": true, "shellhost.exe": true,
	"lockapp.exe": true, "logonui.exe": true, "consent.exe": true,
	"textinputhost.exe": true, "applicationframehost.exe": false, // decided by AUMID below
}

// ExplorerClass is the window class of File Explorer windows; every other explorer.exe
// window is the taskbar, the desktop or the Alt+Tab host.
const ExplorerClass = "cabinetwclass"

// classifyWindow turns what is known about the foreground window into match keys.
// ok=false means "ignore this window and keep the current profile".
func classifyWindow(exeBase, class, aumid string, ownProcess bool) (keys []string, ok bool) {
	exeBase, class, aumid = strings.ToLower(exeBase), strings.ToLower(class), strings.ToLower(aumid)
	switch {
	case ownProcess || exeBase == "" || shellExes[exeBase]:
		return nil, false
	case exeBase == "explorer.exe":
		if class != ExplorerClass {
			return nil, false
		}
		return []string{"class:" + ExplorerClass}, true
	case exeBase == "applicationframehost.exe":
		// Store apps run inside this host; without an AUMID it is a frame in transition.
		if aumid == "" {
			return nil, false
		}
		return []string{"aumid:" + aumid}, true
	}
	keys = []string{"exe:" + exeBase}
	if aumid != "" {
		keys = append(keys, "aumid:"+aumid)
	}
	return keys, true
}

// MatchesAny reports whether the foreground app produces any of the given keys.
func (a ForegroundApp) MatchesAny(keys []string) bool {
	for _, k := range keys {
		for _, ak := range a.Keys {
			if strings.EqualFold(k, ak) {
				return true
			}
		}
	}
	return false
}
