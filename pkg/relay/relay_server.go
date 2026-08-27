package relay

import (
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"isthmus/internal/logger"
)

type clientSession struct {
	deviceID string
	conn     net.Conn
	sendChan chan *Frame
	closed   chan struct{}
	once     sync.Once
}

func (cs *clientSession) Close() {
	cs.once.Do(func() {
		close(cs.closed)
		cs.conn.Close()
	})
}

type Server struct {
	mu       sync.RWMutex
	clients  map[string]*clientSession
	listener net.Listener
	stopChan chan struct{}
	log      *logger.Logger
}

func NewServer() *Server {
	return &Server{
		clients:  make(map[string]*clientSession),
		stopChan: make(chan struct{}),
		log:      logger.WithPrefix("RelayServer"),
	}
}

func (s *Server) ListenAndServe(addr string) error {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to bind relay server on %s: %w", addr, err)
	}

	s.mu.Lock()
	s.listener = l
	s.mu.Unlock()

	s.log.Info("DERP packet relay listening on %s", addr)

	for {
		conn, err := l.Accept()
		if err != nil {
			select {
			case <-s.stopChan:
				return nil
			default:
				s.log.Debug("Relay accept error: %v", err)
				time.Sleep(50 * time.Millisecond)
				continue
			}
		}

		go s.handleConnection(conn)
	}
}

func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()

	// First frame must be handshake
	handshake, err := ReadFrame(conn)
	if err != nil {
		s.log.Debug("Failed to read handshake frame from %s: %v", conn.RemoteAddr(), err)
		return
	}

	if handshake.Type != TypeHandshake || handshake.SourceID == "" {
		s.log.Debug("Invalid handshake from %s", conn.RemoteAddr())
		return
	}

	session := &clientSession{
		deviceID: handshake.SourceID,
		conn:     conn,
		sendChan: make(chan *Frame, 256),
		closed:   make(chan struct{}),
	}

	s.mu.Lock()
	if existing, ok := s.clients[session.deviceID]; ok {
		existing.Close()
		delete(s.clients, session.deviceID)
	}
	s.clients[session.deviceID] = session
	s.mu.Unlock()

	s.log.Info("Device %s connected to relay from %s", session.deviceID, conn.RemoteAddr())

	// Reply with handshake ack
	ack := &Frame{
		Type:     TypeHandshake,
		SourceID: "relay-server",
		TargetID: session.deviceID,
	}
	if err := WriteFrame(conn, ack); err != nil {
		return
	}

	// Writer loop
	go func() {
		for {
			select {
			case <-session.closed:
				return
			case f, ok := <-session.sendChan:
				if !ok {
					return
				}
				if err := WriteFrame(conn, f); err != nil {
					return
				}
			}
		}
	}()

	// Reader & dispatch loop
	defer func() {
		s.mu.Lock()
		if curr, ok := s.clients[session.deviceID]; ok && curr == session {
			delete(s.clients, session.deviceID)
		}
		s.mu.Unlock()
		session.Close()
		s.log.Info("Device %s disconnected from relay", session.deviceID)
	}()

	for {
		frame, err := ReadFrame(conn)
		if err != nil {
			if err != io.EOF {
				s.log.Debug("Read error from %s: %v", session.deviceID, err)
			}
			return
		}

		switch frame.Type {
		case TypeData:
			s.mu.RLock()
			target, exists := s.clients[frame.TargetID]
			s.mu.RUnlock()

			if exists {
				select {
				case target.sendChan <- frame:
				default:
					s.log.Warn("Dropping frame: target %s send buffer full", frame.TargetID)
				}
			} else {
				s.log.Debug("Target device %s not connected to relay", frame.TargetID)
			}

		case TypePing:
			pong := &Frame{
				Type:     TypePong,
				SourceID: "relay-server",
				TargetID: session.deviceID,
			}
			_ = WriteFrame(conn, pong)

		case TypeClose:
			return
		}
	}
}

func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	select {
	case <-s.stopChan:
		return nil
	default:
		close(s.stopChan)
	}

	if s.listener != nil {
		s.listener.Close()
	}

	for _, c := range s.clients {
		close(c.closed)
		c.conn.Close()
	}
	return nil
}
