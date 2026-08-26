package relay

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"testing"
	"time"
)

func getFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func TestFrameSerialization(t *testing.T) {
	original := &Frame{
		Type:     TypeData,
		SourceID: "11111111222222223333333344444444",
		TargetID: "55555555666666667777777788888888",
		Payload:  []byte("hello isthmus relay frame test payload"),
	}

	var buf bytes.Buffer
	if err := WriteFrame(&buf, original); err != nil {
		t.Fatalf("WriteFrame failed: %v", err)
	}

	decoded, err := ReadFrame(&buf)
	if err != nil {
		t.Fatalf("ReadFrame failed: %v", err)
	}

	if decoded.Type != original.Type {
		t.Fatalf("expected type %d, got %d", original.Type, decoded.Type)
	}
	if decoded.SourceID != original.SourceID {
		t.Fatalf("expected source %s, got %s", original.SourceID, decoded.SourceID)
	}
	if decoded.TargetID != original.TargetID {
		t.Fatalf("expected target %s, got %s", original.TargetID, decoded.TargetID)
	}
	if !bytes.Equal(decoded.Payload, original.Payload) {
		t.Fatal("payload mismatch")
	}
}

func TestRelayConnectionEcho(t *testing.T) {
	port, err := getFreePort()
	if err != nil {
		t.Fatalf("Failed to allocate port: %v", err)
	}

	relayServer := NewServer()
	relayAddr := fmt.Sprintf("127.0.0.1:%d", port)

	go func() {
		_ = relayServer.ListenAndServe(relayAddr)
	}()
	defer relayServer.Close()

	time.Sleep(50 * time.Millisecond)

	nodeAID := "node-a-012345678901234567890123"
	nodeBID := "node-b-012345678901234567890123"

	clientB, err := ConnectRelay(relayAddr, nodeBID)
	if err != nil {
		t.Fatalf("ConnectRelay for node B failed: %v", err)
	}
	defer clientB.Close()

	clientA, err := ConnectRelay(relayAddr, nodeAID)
	if err != nil {
		t.Fatalf("ConnectRelay for node A failed: %v", err)
	}
	defer clientA.Close()

	// Node B starts echo listener
	go func() {
		connB, err := clientB.Accept()
		if err != nil {
			return
		}
		defer connB.Close()

		buf := make([]byte, 64*1024)
		for {
			n, err := connB.Read(buf)
			if n > 0 {
				_, _ = connB.Write(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()

	// Node A dials Node B through relay
	connA, err := clientA.DialPeer(nodeBID)
	if err != nil {
		t.Fatalf("DialPeer failed: %v", err)
	}
	defer connA.Close()

	testData := make([]byte, 64*1024)
	if _, err := io.ReadFull(rand.Reader, testData); err != nil {
		t.Fatal(err)
	}

	if _, err := connA.Write(testData); err != nil {
		t.Fatalf("connA.Write failed: %v", err)
	}

	receivedData := make([]byte, len(testData))
	if _, err := io.ReadFull(connA, receivedData); err != nil {
		t.Fatalf("connA.Read failed: %v", err)
	}

	if !bytes.Equal(receivedData, testData) {
		t.Fatal("echoed data through relay does not match sent data")
	}
}
