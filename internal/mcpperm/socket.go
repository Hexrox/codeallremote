package mcpperm

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"os"
	"sync"
	"time"
)

// PermissionRequest is carried from the car-mcp-perm helper to the CAR adapter
// over the unix-socket transport.
type PermissionRequest struct {
	Session   string          `json:"session"`
	ToolName  string          `json:"tool_name"`
	ToolUseID string          `json:"tool_use_id"`
	Input     json.RawMessage `json:"input"`
}

// socketReply is the wire-level reply sent back from the adapter to the helper.
type socketReply struct {
	Allow   bool   `json:"allow"`
	Message string `json:"message"`
}

const failClosedMessage = "CAR: approval unavailable (fail-closed)"

func failClosed() Decision {
	return Decision{Allow: false, Message: failClosedMessage}
}

// DecideOverSocket dials the unix socket, sends a single JSON-line request,
// reads a single JSON-line reply, and returns the decision. On ANY error it
// returns a fail-closed deny.
func DecideOverSocket(socketPath string, req PermissionRequest, timeout time.Duration) Decision {
	conn, err := net.DialTimeout("unix", socketPath, timeout)
	if err != nil {
		return failClosed()
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return failClosed()
	}

	data, err := json.Marshal(req)
	if err != nil {
		return failClosed()
	}
	if _, err := conn.Write(append(data, '\n')); err != nil {
		return failClosed()
	}

	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return failClosed()
	}

	var reply socketReply
	if err := json.Unmarshal(line, &reply); err != nil {
		return failClosed()
	}

	return Decision{Allow: reply.Allow, Message: reply.Message}
}

// SocketServer listens on a unix socket and dispatches permission requests
// to the provided handler.
type SocketServer struct {
	ln      net.Listener
	handler func(PermissionRequest) Decision
	mu      sync.Mutex
	closed  bool
}

// NewSocketServer removes any stale socket file and creates a new listener.
func NewSocketServer(socketPath string, handler func(PermissionRequest) Decision) (*SocketServer, error) {
	_ = os.Remove(socketPath)
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}
	return &SocketServer{ln: ln, handler: handler}, nil
}

// Serve runs the accept loop until ctx is cancelled or the listener is closed.
func (s *SocketServer) Serve(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		s.Close()
	}()

	for {
		conn, err := s.ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				return err
			}
		}
		go s.handle(conn)
	}
}

func (s *SocketServer) handle(conn net.Conn) {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return
	}

	var req PermissionRequest
	if err := json.Unmarshal(line, &req); err != nil {
		return
	}

	var dec Decision
	func() {
		defer func() {
			if r := recover(); r != nil {
				dec = failClosed()
			}
		}()
		dec = s.handler(req)
	}()

	reply := socketReply{Allow: dec.Allow, Message: dec.Message}
	data, err := json.Marshal(reply)
	if err != nil {
		return
	}
	conn.Write(append(data, '\n'))
}

// Addr returns the socket path.
func (s *SocketServer) Addr() string {
	return s.ln.Addr().String()
}

// Close closes the underlying listener. Safe to call multiple times.
func (s *SocketServer) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return s.ln.Close()
}
