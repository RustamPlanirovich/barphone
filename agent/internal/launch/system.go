package launch

import "time"

// SystemAction is something a "system" button does on the PC.
type SystemAction struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Confirm bool   `json:"confirm,omitempty"` // phone must ask before pressing
	Slider  bool   `json:"slider,omitempty"`  // hold and drag to set a level
	Swipe   bool   `json:"swipe,omitempty"`   // the virtual desktops tile: swipe to switch
	// Deferred actions run shortly after the press is answered: the PC may lock, sleep
	// or switch off right away, and the phone should still see the result.
	Deferred bool `json:"-"`
}

var SystemActions = []SystemAction{
	{ID: "media_play_pause", Title: "Пауза / воспроизведение"},
	{ID: "media_next", Title: "Следующий трек"},
	{ID: "media_prev", Title: "Предыдущий трек"},
	{ID: "media_stop", Title: "Стоп"},
	{ID: "volume", Title: "Громкость", Slider: true},
	{ID: "volume_up", Title: "Громче"},
	{ID: "volume_down", Title: "Тише"},
	{ID: "mute", Title: "Без звука"},
	{ID: "brightness", Title: "Яркость", Slider: true},
	{ID: "brightness_up", Title: "Ярче"},
	{ID: "brightness_down", Title: "Темнее"},
	{ID: "desktops", Title: "Рабочие столы", Swipe: true},
	{ID: "desktop_next", Title: "Следующий рабочий стол"},
	{ID: "desktop_prev", Title: "Предыдущий рабочий стол"},
	{ID: "task_view", Title: "Обзор окон и столов"},
	{ID: "lock", Title: "Заблокировать", Deferred: true},
	{ID: "sleep", Title: "Сон", Deferred: true},
	{ID: "display_off", Title: "Выключить экран", Deferred: true},
	{ID: "shutdown", Title: "Выключить", Confirm: true, Deferred: true},
	{ID: "restart", Title: "Перезагрузить", Confirm: true, Deferred: true},
}

func LookupSystemAction(id string) (SystemAction, bool) {
	for _, a := range SystemActions {
		if a.ID == id {
			return a, true
		}
	}
	return SystemAction{}, false
}

// DesktopInfo describes the virtual desktops of the PC for the phone's tile.
type DesktopInfo struct {
	Count   int      `json:"count"`
	Current int      `json:"current"` // from 0
	Names   []string `json:"names"`   // "" = not renamed
}

// CallInfo is a video call in progress (Google Meet), for the phone's call controls.
type CallInfo struct {
	Active bool   `json:"active"`
	App    string `json:"app,omitempty"`
	Camera bool   `json:"camera"` // the camera is in use (Meet releases it when video is off)
}

// VolumeState is the PC's master output volume.
type VolumeState struct {
	Level float64 `json:"value"` // 0..1
	Muted bool    `json:"muted"`
}

const deferDelay = 400 * time.Millisecond

// brightnessStep is what "Ярче" and "Темнее" change.
const brightnessStep = 0.1

// stepBrightness moves the brightness one step; a slider tap (dir 0) changes nothing.
func stepBrightness(l Launcher, dir float64) error {
	cur, err := l.Brightness()
	if err != nil || dir == 0 {
		return err
	}
	return l.SetBrightness(min(max(cur+dir*brightnessStep, 0), 1))
}

// brightnessAction handles the brightness system actions; ok is false for other IDs.
func brightnessAction(l Launcher, id string) (err error, ok bool) {
	switch id {
	case "brightness":
		return stepBrightness(l, 0), true
	case "brightness_up":
		return stepBrightness(l, 1), true
	case "brightness_down":
		return stepBrightness(l, -1), true
	}
	return nil, false
}

// MaxTextLen bounds a "text" button.
const MaxTextLen = 2000
