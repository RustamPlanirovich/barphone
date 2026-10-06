package launch

import (
	"reflect"
	"testing"
)

func TestClassifyWindow(t *testing.T) {
	for _, tc := range []struct {
		name              string
		exe, class, aumid string
		own               bool
		want              []string // nil = ignored, keep the current profile
	}{
		{"desktop app", "Code.exe", "Chrome_WidgetWin_1", "", false, []string{"exe:code.exe"}},
		{"desktop app with AUMID", "Telegram.exe", "Qt5QWindow", "TelegramDesktop", false, []string{"exe:telegram.exe", "aumid:telegramdesktop"}},
		{"File Explorer", "explorer.exe", "CabinetWClass", "", false, []string{"class:cabinetwclass"}},
		{"taskbar", "explorer.exe", "Shell_TrayWnd", "", false, nil},
		{"desktop", "explorer.exe", "Progman", "", false, nil},
		{"Alt+Tab host", "explorer.exe", "XamlExplorerHostIslandWindow", "", false, nil},
		{"Store app", "ApplicationFrameHost.exe", "ApplicationFrameWindow", "Microsoft.WindowsCalculator_8wekyb3d8bbwe!App", false,
			[]string{"aumid:microsoft.windowscalculator_8wekyb3d8bbwe!app"}},
		{"Store frame in transition", "ApplicationFrameHost.exe", "ApplicationFrameWindow", "", false, nil},
		{"Start menu", "StartMenuExperienceHost.exe", "Windows.UI.Core.CoreWindow", "", false, nil},
		{"search", "SearchHost.exe", "Windows.UI.Core.CoreWindow", "", false, nil},
		{"lock screen", "LockApp.exe", "Windows.UI.Core.CoreWindow", "", false, nil},
		{"notification area", "ShellExperienceHost.exe", "Windows.UI.Core.CoreWindow", "", false, nil},
		{"the agent itself", "barphone-agent.exe", "x", "", true, nil},
		{"unknown process", "", "x", "", false, nil},
	} {
		keys, ok := classifyWindow(tc.exe, tc.class, tc.aumid, tc.own)
		if tc.want == nil {
			if ok {
				t.Errorf("%s: should be ignored, got %v", tc.name, keys)
			}
			continue
		}
		if !ok || !reflect.DeepEqual(keys, tc.want) {
			t.Errorf("%s: got %v %v, want %v", tc.name, keys, ok, tc.want)
		}
	}
}

func TestForegroundMatchesBinding(t *testing.T) {
	code := ForegroundApp{Keys: []string{"exe:code.exe"}}
	if !code.MatchesAny([]string{"aumid:microsoft.visualstudiocode", "EXE:Code.exe"}) {
		t.Error("a binding made from the Start menu entry must match the running window")
	}
	if code.MatchesAny([]string{"exe:codium.exe"}) {
		t.Error("different exe")
	}
	if (ForegroundApp{}).MatchesAny([]string{"exe:code.exe"}) {
		t.Error("nothing in front matches nothing")
	}
}
