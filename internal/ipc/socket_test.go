package ipc

import (
	"path/filepath"
	"testing"
	"time"
)

func TestIPCToggleStop(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "wiwispr-test.sock")

	// 1. When no server is running, TryStopExisting returns false
	stopped, err := TryStopExisting(sock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stopped {
		t.Fatal("expected stopped=false when no server is running")
	}

	// 2. Start server
	srv, err := StartServer(sock)
	if err != nil {
		t.Fatalf("StartServer failed: %v", err)
	}
	defer srv.Close()

	// 3. Second instance calls TryStopExisting -> should return true and close srv.StopChan()
	stopped, err = TryStopExisting(sock)
	if err != nil {
		t.Fatalf("TryStopExisting failed: %v", err)
	}
	if !stopped {
		t.Fatal("expected stopped=true when active server is running")
	}

	select {
	case <-srv.StopChan():
		// Success!
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for StopChan to close")
	}
}
