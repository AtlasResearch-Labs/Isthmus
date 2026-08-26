package relay

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"isthmus/internal/logger"
)

type RelayAddr struct {
	DeviceID string
}

func (a RelayAddr) Network() string { return "isthmus-relay" }
func (a RelayAddr) String() string  { return a.DeviceID }

type RelayClient struct {
	mu           sync.RWMutex
	relayAddr    string
	localID      string
	conn         net.Conn
	writeMu      sync.Mutex
	sessions     map[string]*RelayConn
	incomingChan chan *RelayConn
	closed       chan struct{}
	log          *logger.Logger
}

func ConnectRelay(relayAddr, localID string) (*RelayClient, error) {
	conn, err := net.DialTimeout("tcp", relayAddr, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to dial relay %s: %w", relayAddr, err)
	}

	handshake := &Frame{
		Type:     TypeHandshake,
		SourceID: localID,
		TargetID: "relay-server",
	}

	if err := WriteFrame(conn, handshake); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to send handshake to relay: %w", err)
	}

	ack, err := ReadFrame(conn)
	if err != nil || ack.Type != TypeHandshake {
		conn.Close()
		return nil, fmt.Errorf("invalid handshake ack from relay: %v", err)
	}

	client := &RelayClient{
		relayAddr:    relayAddr,
		localID:      localID,
		conn:         conn,
		sessions:     make(map[string]*RelayConn),
		incomingChan: make(chan *RelayConn, 64),
		closed:       make(chan struct{}),
		log:          logger.WithPrefix("RelayClient"),
	}

	go client.readLoop()
	return client, nil
}

func (c *RelayClient) readLoop() {
	defer func() {
		close(c.closed)
		c.conn.Close()
	}()

	for {
		frame, err := ReadFrame(c.conn)
		if err != nil {
			return
		}

		if frame.Type == TypeData {
			c.mu.Lock()
			session, ok := c.sessions[frame.SourceID]
			if !ok {
				session = newRelayConn(c, frame.SourceID)
				c.sessions[frame.SourceID] = session
				select {
				case c.incomingChan <- session:
				default:
				}
			}
			c.mu.Unlock()

			session.pushData(frame.Payload)
		}
	}
}

func (c *RelayClient) DialPeer(targetID string) (net.Conn, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if session, ok := c.sessions[targetID]; ok && !session.isClosed {
		return session, nil
	}

	session := newRelayConn(c, targetID)
	c.sessions[targetID] = session
	return session, nil
}

func (c *RelayClient) Accept() (net.Conn, error) {
	select {
	case session := <-c.incomingChan:
		return session, nil
	case <-c.closed:
		return nil, io.EOF
	}
}

func (c *RelayClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.Close()
}

// RelayConn implements net.Conn
type RelayConn struct {
	client    *RelayClient
	peerID    string
	readBuf   bytes.Buffer
	readMu    sync.Mutex
	dataCond  *sync.Cond
	isClosed  bool
	closeChan chan struct{}
}

func newRelayConn(client *RelayClient, peerID string) *RelayConn {
	rc := &RelayConn{
		client:    client,
		peerID:    peerID,
		closeChan: make(chan struct{}),
	}
	rc.dataCond = sync.NewCond(&rc.readMu)
	return rc
}

func (c *RelayConn) pushData(data []byte) {
	c.readMu.Lock()
	defer c.readMu.Unlock()

	c.readBuf.Write(data)
	c.dataCond.Broadcast()
}

func (c *RelayConn) Read(b []byte) (n int, err error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()

	for c.readBuf.Len() == 0 {
		if c.isClosed {
			return 0, io.EOF
		}
		c.dataCond.Wait()
	}

	return c.readBuf.Read(b)
}

func (c *RelayConn) Write(b []byte) (n int, err error) {
	if c.isClosed {
		return 0, io.ErrClosedPipe
	}

	frame := &Frame{
		Type:     TypeData,
		SourceID: c.client.localID,
		TargetID: c.peerID,
		Payload:  b,
	}

	c.client.writeMu.Lock()
	defer c.client.writeMu.Unlock()

	if err := WriteFrame(c.client.conn, frame); err != nil {
		return 0, err
	}
	return len(b), nil
}

func (c *RelayConn) Close() error {
	c.readMu.Lock()
	defer c.readMu.Unlock()

	if !c.isClosed {
		c.isClosed = true
		c.dataCond.Broadcast()
	}
	return nil
}

func (c *RelayConn) LocalAddr() net.Addr                { return RelayAddr{DeviceID: c.client.localID} }
func (c *RelayConn) RemoteAddr() net.Addr               { return RelayAddr{DeviceID: c.peerID} }
func (c *RelayConn) SetDeadline(t time.Time) error      { return nil }
func (c *RelayConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *RelayConn) SetWriteDeadline(t time.Time) error { return nil }
