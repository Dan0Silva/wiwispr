package ui

import (
	_ "embed"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

//go:embed overlay.py
var overlayScript []byte

// Overlay controls the floating non-focus sound-wave indicator process.
type Overlay struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	mu    sync.Mutex
}

// Start launches the floating GTK/Cairo overlay subprocess if Python3 + GTK3 are available.
// If disabled or unavailable, it returns a no-op Overlay so recording still works seamlessly.
func Start(disabled bool) *Overlay {
	if disabled {
		return &Overlay{}
	}

	pythonBin, err := exec.LookPath("python3")
	if err != nil {
		return &Overlay{}
	}

	scriptPath := filepath.Join(os.TempDir(), fmt.Sprintf("wiwispr-overlay-%d.py", os.Getuid()))
	if err := os.WriteFile(scriptPath, overlayScript, 0700); err != nil {
		return &Overlay{}
	}

	cmd := exec.Command(pythonBin, scriptPath)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return &Overlay{}
	}

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return &Overlay{}
	}

	return &Overlay{
		cmd:   cmd,
		stdin: stdin,
	}
}

// UpdateLevel sends the current normalized microphone level (0.0..1.0) and speech state to the overlay.
func (o *Overlay) UpdateLevel(level float64, speaking bool) {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.stdin == nil {
		return
	}
	spkFlag := 0
	if speaking {
		spkFlag = 1
	}
	_, _ = fmt.Fprintf(o.stdin, "LEVEL %.3f %d\n", level, spkFlag)
}

// SetTranscribing switches the overlay animation into the blue pulsing "Transcribing" wave state.
func (o *Overlay) SetTranscribing() {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.stdin == nil {
		return
	}
	_, _ = fmt.Fprintln(o.stdin, "STATE TRANSCRIBING")
}

// Close cleanly terminates the overlay window immediately.
func (o *Overlay) Close() {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.stdin != nil {
		_, _ = fmt.Fprintln(o.stdin, "QUIT")
		_ = o.stdin.Close()
		o.stdin = nil
	}
	if o.cmd != nil && o.cmd.Process != nil {
		_ = o.cmd.Process.Kill()
		_ = o.cmd.Wait()
		o.cmd = nil
	}
}
