package gather

type RunState string

const (
	RunStateIdle        RunState = "idle"
	RunStateCalibrating RunState = "calibrating"
	RunStateRunning     RunState = "running"
	RunStatePaused      RunState = "paused"
	RunStateStopped     RunState = "stopped"
	RunStateError       RunState = "error"
)

type StepMode string

const (
	StepModeMove     StepMode = "moving"
	StepModeInteract StepMode = "interacting"
)

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type WindowRect struct {
	Left   int `json:"left"`
	Top    int `json:"top"`
	Right  int `json:"right"`
	Bottom int `json:"bottom"`
}

func (r WindowRect) Width() int {
	return r.Right - r.Left
}

func (r WindowRect) Height() int {
	return r.Bottom - r.Top
}

func (r WindowRect) IsZero() bool {
	return r.Left == 0 && r.Top == 0 && r.Right == 0 && r.Bottom == 0
}

type CalibrationPayload struct {
	WindowRect         WindowRect `json:"windowRect"`
	CenterOffsetX      int        `json:"centerOffsetX"`
	CenterOffsetY      int        `json:"centerOffsetY"`
	SafeMarginX        int        `json:"safeMarginX"`
	SafeMarginY        int        `json:"safeMarginY"`
	MoveRadiusPx       int        `json:"moveRadiusPx"`
	InteractRadiusPx   int        `json:"interactRadiusPx"`
	InteractThresholdG int        `json:"interactThresholdGu"`
	TitleHint          string     `json:"titleHint,omitempty"`
	DesiredMount       bool       `json:"desiredMount"`
	MountKey           string     `json:"mountKey,omitempty"`
	// Animation timings. Ox mounts are much slower than horses — defaults are
	// tuned for ox; speedy mounts can drop them via the UI.
	MountDelayMs    int `json:"mountDelayMs,omitempty"`
	DismountDelayMs int `json:"dismountDelayMs,omitempty"`
	// ClickRefreshMs is how long a movement click is considered "still valid"
	// before we re-click at the same approximate spot. While we're inside the
	// refresh window and the click position hasn't drifted, we skip the click
	// so Albion keeps walking smoothly instead of restarting the path every tick.
	ClickRefreshMs int `json:"clickRefreshMs,omitempty"`
	// ClickDriftPx is the pixel distance between the current click and the last
	// click beyond which we force a re-click even within the refresh window —
	// i.e. the direction changed enough that we need to steer.
	ClickDriftPx int `json:"clickDriftPx,omitempty"`
}

type GatherTarget struct {
	ID         int64   `json:"id"`
	Type       string  `json:"type"`
	Tier       int     `json:"tier"`
	Enchant    int     `json:"enchant"`
	Size       int     `json:"size"`
	IsLiving   bool    `json:"isLiving"`
	Position   Point   `json:"position"`
	Distance   float64 `json:"distance"`
	Status     string  `json:"status"`
	LastSeenAt int64   `json:"lastSeenAt"`
}

type StepResult struct {
	Mode          StepMode `json:"mode"`
	ClickX        int      `json:"clickX"`
	ClickY        int      `json:"clickY"`
	Distance      float64  `json:"distance"`
	FocusedWindow bool     `json:"focusedWindow"`
	Message       string   `json:"message"`
	// KeyTap, when non-empty, tells the executor to press a single key (e.g. "A"
	// for mount toggle) instead of — or in addition to — the right-click.
	KeyTap string `json:"keyTap,omitempty"`
	// SkipClick suppresses the mouse click for this step. Used when a KeyTap is
	// the only action we want (e.g. mount up before moving).
	SkipClick bool `json:"skipClick,omitempty"`
	// HoldMove, when true on a Move step, tells the executor to press the right
	// mouse button down (if not already) and keep it held across ticks.
	// Albion interprets a held right-click as "follow the cursor," giving smooth
	// continuous travel instead of the start-stop cadence of one click per tick.
	// The executor releases the hold on any non-Move step or on explicit stop.
	HoldMove bool `json:"holdMove,omitempty"`
	// ReleaseHold forces the executor to lift a held right-click before doing
	// anything else this step. Set for Interact steps, mount/dismount taps, and
	// when the gather loop stops.
	ReleaseHold bool `json:"releaseHold,omitempty"`
}

type GatherStatusResponse struct {
	Supported      bool                `json:"supported"`
	Platform       string              `json:"platform"`
	Active         bool                `json:"active"`
	State          RunState            `json:"state"`
	Calibrated     bool                `json:"calibrated"`
	FocusedWindow  bool                `json:"focusedWindow"`
	CurrentTarget  *GatherTarget       `json:"currentTarget,omitempty"`
	LastError      string              `json:"lastError,omitempty"`
	LastAction     string              `json:"lastAction,omitempty"`
	Calibration    *CalibrationPayload `json:"calibration,omitempty"`
	ConsecutiveErr int                 `json:"consecutiveErrors"`
}

type GatherCommandRequest struct {
	Active       *bool               `json:"active,omitempty"`
	Player       *Point              `json:"player,omitempty"`
	Target       *GatherTarget       `json:"target,omitempty"`
	Calibration  *CalibrationPayload `json:"calibration,omitempty"`
	Reason       string              `json:"reason,omitempty"`
	CurrentMapID string              `json:"currentMapId,omitempty"`
	IsMounted    bool                `json:"isMounted,omitempty"`
	// TestDirection, when non-zero, overrides target computation to step towards
	// one of the four cardinal world directions. Used by the debug "test click"
	// buttons to verify the projection is aligned. Accepted values: "N","E","S","W".
	TestDirection string `json:"testDirection,omitempty"`
}

type GatherCommandResponse struct {
	Status GatherStatusResponse `json:"status"`
	Step   *StepResult          `json:"step,omitempty"`
}
