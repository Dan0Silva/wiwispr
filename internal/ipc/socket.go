package ipc

import (
	"errors"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

// TryStopExisting checks if an instance of wiwispr is already running on socketPath.
// If an active instance responds, it sends the "STOP" command and returns (true, nil),
// meaning the caller should exit immediately because it toggled the active session.
// If no active instance is running (or socket is stale), it cleans up the stale socket
// and returns (false, nil).
func TryStopExisting(socketPath string) (bool, error) {
	conn, err := net.DialTimeout("unix", socketPath, 250*time.Millisecond)
	if err != nil {
		// If socket file exists on disk but nobody is listening, remove stale socket
		if _, statErr := os.Stat(socketPath); statErr == nil {
			_ = os.Remove(socketPath)
		}
		return false, nil
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(500 * time.Millisecond))
	if _, err := conn.Write([]byte("STOP\n")); err != nil {
		return false, err
	}

	buf := make([]byte, 16)
	n, _ := conn.Read(buf)
	resp := strings.TrimSpace(string(buf[:n]))
	if resp == "OK" {
		return true, nil
	}
	return true, nil
}

// Server listens on a UNIX domain socket for a toggle "STOP" signal.
type Server struct {
	socketPath string
	listener   net.Listener
	stopCh     chan struct{}
	once       sync.Once
}

// StartServer creates the UNIX socket listener for the active recording session.
func StartServer(socketPath string) (*Server, error) {
	_ = os.Remove(socketPath)
	l, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}
	_ = os.Chmod(socketPath, 0600)

	s := &Server{
		socketPath: socketPath,
		listener:   l,
		stopCh:     make(chan struct{}),
	}

	go s.acceptLoop()
	return s, nil
}

func (s *Server) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			continue
		}
		go s.handleConn(conn)
	}
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(500 * time.Millisecond))
	buf := make([]byte, 32)
	n, err := conn.Read(buf)
	if err != nil && !errors.Is(err, io.EOF) {
		return
	}
	cmd := strings.TrimSpace(string(buf[:n]))
	if cmd == "STOP" {
		_, _ = conn.Write([]byte("OK\n"))
		s.once.Do(func() {
			close(s.stopCh)
		})
	}
}

// StopChan returns a channel that is closed when another wiwispr invocation sends "STOP".
func (s *Server) StopChan() <-chan struct{} {
	return s.stopCh
}

// Close stops the UNIX socket server and removes the socket file.
func (s *Server) Close() {
	if s.listener != nil {
		_ = s.listener.Close()
	}
	_ = os.Remove(s.socketPath)
}
