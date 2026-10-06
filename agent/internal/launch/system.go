package launch

import "time"

// SystemAction is something a "system" button does on the PC.
type SystemAction struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Confirm bool   `json:"confirm,omitempty"` // phone must ask before pressing
	Slider  bool   `json:"slider,omitempty"`  // hold and drag to set a level
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

// VolumeState is the PC's master output volume.
type VolumeState struct {
	Level float64 `json:"value"` // 0..1
	Muted bool    `json:"muted"`
}

const deferDelay = 400 * time.Millisecond

// MaxTextLen bounds a "text" button.
const MaxTextLen = 2000
