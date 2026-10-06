//go:build windows

package launch

import "testing"

func TestMeetTitle(t *testing.T) {
	for title, want := range map[string]bool{
		"Meet\u00a0– qgh-ehxh-jmi - Google Chrome":       true, // as Chrome shows it in a call
		"Meet - abc-defg-hij":                            true, // the Meet app window
		"Google Meet – Weekly sync":                      true,
		"Meet\u2009—\u2009abc-defg-hij - Microsoft Edge": true,
		"Meet":                         false, // the Meet home page, no call
		"Meeting notes.docx - Word":    false,
		"meetbot - Visual Studio Code": false,
	} {
		if got := meetTitle.MatchString(title); got != want {
			t.Errorf("%q: %v, want %v", title, got, want)
		}
	}
}
