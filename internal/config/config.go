package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Config holds the runtime configuration for wiwispr.
type Config struct {
	// WhisperBinary is the path to whisper-cli executable.
	WhisperBinary string
	// ModelPath is the path to the ggml model file (.bin).
	ModelPath string
	// Language is the target spoken language (default: "pt").
	Language string
	// Threads is the number of CPU threads for whisper-cli.
	Threads int

	// SilenceDuration is how long silence must persist after speech to auto-stop (default: 2s).
	SilenceDuration time.Duration
	// MaxInitialSilence is the timeout if the user triggers wiwispr but says nothing (default: 7s).
	MaxInitialSilence time.Duration
	// MaxRecordingDuration is the safety cap for a single recording (default: 120s).
	MaxRecordingDuration time.Duration
	// SpeechThresholdRMS is the normalized RMS threshold (0.0 to 1.0) that counts as voice activity.
	SpeechThresholdRMS float64

	// TypingDelay is the delay between simulated keystrokes for the typewriter effect.
	TypingDelay time.Duration
	// KeyboardLayout specifies the layout for uinput typing ("us-intl" or "abnt2").
	KeyboardLayout string

	// SocketPath is the UNIX domain socket path used to toggle an active session.
	SocketPath string
	// TempWAVPath is the temporary audio file path (placed in /dev/shm RAM disk by default).
	TempWAVPath string
	// NoOverlay disables the floating visual indicator when true.
	NoOverlay bool
}

// DefaultConfig returns sensible defaults tailored for Fedora KDE Wayland in Portuguese.
func DefaultConfig() Config {
	home, _ := os.UserHomeDir()
	dataDir := filepath.Join(home, ".local", "share", "wiwispr")
	binDir := filepath.Join(home, ".local", "bin")

	uid := os.Getuid()
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = fmt.Sprintf("/run/user/%d", uid)
	}

	shmDir := "/dev/shm"
	if st, err := os.Stat(shmDir); err != nil || !st.IsDir() {
		shmDir = os.TempDir()
	}

	whisperBin := filepath.Join(binDir, "whisper-cli")
	if envBin := os.Getenv("WIWISPR_WHISPER_BIN"); envBin != "" {
		whisperBin = envBin
	}

	modelPath := filepath.Join(dataDir, "models", "ggml-small.bin")
	if envModel := os.Getenv("WIWISPR_MODEL"); envModel != "" {
		modelPath = envModel
	} else if _, err := os.Stat(modelPath); err != nil {
		// Fallback to base if small is not yet downloaded
		basePath := filepath.Join(dataDir, "models", "ggml-base.bin")
		if _, errBase := os.Stat(basePath); errBase == nil {
			modelPath = basePath
		}
	}

	lang := "pt"
	if envLang := os.Getenv("WIWISPR_LANG"); envLang != "" {
		lang = envLang
	}

	silenceSec := 2.0
	if envSil := os.Getenv("WIWISPR_SILENCE_SEC"); envSil != "" {
		if v, err := strconv.ParseFloat(envSil, 64); err == nil && v >= 0.4 {
			silenceSec = v
		}
	}

	threshold := 0.018
	if envThr := os.Getenv("WIWISPR_THRESHOLD"); envThr != "" {
		if v, err := strconv.ParseFloat(envThr, 64); err == nil && v > 0.001 && v < 0.5 {
			threshold = v
		}
	}

	typingDelayMs := 12
	if envDelay := os.Getenv("WIWISPR_TYPING_DELAY_MS"); envDelay != "" {
		if v, err := strconv.Atoi(envDelay); err == nil && v >= 1 && v <= 200 {
			typingDelayMs = v
		}
	}

	layout := "us-intl"
	if envLayout := os.Getenv("WIWISPR_KB_LAYOUT"); envLayout != "" {
		layout = envLayout
	}

	return Config{
		WhisperBinary:        whisperBin,
		ModelPath:            modelPath,
		Language:             lang,
		Threads:              6,
		SilenceDuration:      time.Duration(silenceSec * float64(time.Second)),
		MaxInitialSilence:    7 * time.Second,
		MaxRecordingDuration: 120 * time.Second,
		SpeechThresholdRMS:   threshold,
		TypingDelay:          time.Duration(typingDelayMs) * time.Millisecond,
		KeyboardLayout:       layout,
		SocketPath:           filepath.Join(runtimeDir, "wiwispr.sock"),
		TempWAVPath:          filepath.Join(shmDir, fmt.Sprintf("wiwispr-%d.wav", uid)),
		NoOverlay:            os.Getenv("WIWISPR_NO_OVERLAY") == "1",
	}
}
