package pairing

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	qrcode "github.com/skip2/go-qrcode"

	"isthmus/internal/logger"
	"isthmus/pkg/config"
)

type PairingPayload struct {
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device_name"`
	PublicKey  string `json:"public_key"`
	VirtualIP  string `json:"virtual_ip"`
	Endpoint   string `json:"endpoint,omitempty"`
	Token      string `json:"token"`
}

type PairingSession struct {
	PIN         string    `json:"pin"`
	QRURL       string    `json:"qr_url"`
	QRBase64PNG string    `json:"qr_base64_png"`
	ASCIIQR     string    `json:"ascii_qr,omitempty"`
	Port        int       `json:"port"`
	ExpiresAt   time.Time `json:"expires_at"`

	server    *http.Server
	resultChan chan *config.Peer
	mu        sync.Mutex
}

type Manager struct {
	log *logger.Logger
}

func NewManager() *Manager {
	return &Manager{
		log: logger.WithPrefix("PairingManager"),
	}
}

// GenerateSession creates an ephemeral pairing session with a 6-digit PIN and QR code
func (m *Manager) GenerateSession(cfg *config.Config, hostIP string, ttl time.Duration) (*PairingSession, error) {
	if ttl == 0 {
		ttl = 3 * time.Minute
	}

	// 1. Generate 6-digit numeric PIN (e.g. 583921)
	pinNum, err := rand.Int(rand.Reader, big.NewInt(900000))
	if err != nil {
		return nil, fmt.Errorf("failed to generate random PIN: %w", err)
	}
	pin := fmt.Sprintf("%06d", pinNum.Int64()+100000)

	// 2. Generate random session secret token
	tokenBytes := make([]byte, 16)
	_, _ = rand.Read(tokenBytes)
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)

	// 3. Find free local port for ephemeral pairing listener
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		return nil, fmt.Errorf("failed to open ephemeral pairing listener: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	if hostIP == "" {
		hostIP = getLocalIP()
	}

	endpoint := fmt.Sprintf("%s:%d", hostIP, port)

	// 4. Construct pairing QR URL
	qrURL := fmt.Sprintf("isthmus://pair?id=%s&name=%s&key=%s&ip=%s&ep=%s&pin=%s&tok=%s",
		cfg.DeviceID,
		cfg.DeviceName,
		cfg.PublicKey,
		cfg.VirtualIP,
		endpoint,
		pin,
		token,
	)

	// 5. Generate Base64 PNG QR for GUI and ASCII QR for CLI
	pngBytes, err := qrcode.Encode(qrURL, qrcode.Medium, 256)
	var qrBase64 string
	if err == nil {
		qrBase64 = "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes)
	}

	asciiQR := ""
	qrObj, err := qrcode.New(qrURL, qrcode.Medium)
	if err == nil {
		asciiQR = qrObj.ToSmallString(false)
	}

	session := &PairingSession{
		PIN:         pin,
		QRURL:       qrURL,
		QRBase64PNG: qrBase64,
		ASCIIQR:     asciiQR,
		Port:        port,
		ExpiresAt:   time.Now().Add(ttl),
		resultChan:  make(chan *config.Peer, 1),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/pair", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var clientPayload PairingPayload
		if err := json.NewDecoder(r.Body).Decode(&clientPayload); err != nil {
			http.Error(w, "Invalid payload", http.StatusBadRequest)
			return
		}

		if clientPayload.Token != token && clientPayload.Token != pin {
			http.Error(w, "Invalid pairing token", http.StatusUnauthorized)
			return
		}

		// Save incoming peer
		newPeer := config.Peer{
			DeviceID:         clientPayload.DeviceID,
			DeviceName:       clientPayload.DeviceName,
			PublicKey:        clientPayload.PublicKey,
			VirtualIP:        clientPayload.VirtualIP,
			LastSeenEndpoint: clientPayload.Endpoint,
			LastSeenTime:     time.Now(),
			Allowed:          true,
			ACL: config.PeerACL{
				AllowRead:    true,
				AllowWrite:   true,
				BlockedPaths: []string{".ssh", ".git", ".env", "credentials"},
			},
		}

		_ = cfg.AddPeer(newPeer)
		_ = cfg.Save("")

		// Respond with host credentials
		resp := PairingPayload{
			DeviceID:   cfg.DeviceID,
			DeviceName: cfg.DeviceName,
			PublicKey:  cfg.PublicKey,
			VirtualIP:  cfg.VirtualIP,
			Endpoint:   fmt.Sprintf("%s:%d", hostIP, cfg.SFTPPort),
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)

		select {
		case session.resultChan <- &newPeer:
		default:
		}
	})

	session.server = &http.Server{
		Handler: mux,
	}

	go func() {
		_ = session.server.Serve(listener)
	}()

	// Start UDP broadcast responder for PIN discovery
	go m.runBroadcastResponder(session, pin, token, port)

	return session, nil
}

func (s *PairingSession) Close() {
	if s.server != nil {
		_ = s.server.Close()
	}
}

func (m *Manager) WaitForPairing(ctx context.Context, session *PairingSession) (*config.Peer, error) {
	defer session.Close()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case peer := <-session.resultChan:
		return peer, nil
	case <-time.After(time.Until(session.ExpiresAt)):
		return nil, fmt.Errorf("pairing session expired")
	}
}

// JoinPairing connects to a host via 6-digit PIN or QR payload
func (m *Manager) JoinPairing(ctx context.Context, cfg *config.Config, target string) (*config.Peer, error) {
	target = strings.TrimSpace(target)

	var endpoint, token, expectedName string

	if strings.HasPrefix(target, "isthmus://pair") {
		// Parse QR URL
		vals := parseQuery(target)
		endpoint = vals["ep"]
		token = vals["tok"]
		if token == "" {
			token = vals["pin"]
		}
		expectedName = vals["name"]
	} else {
		// Numeric PIN mode: discover host over LAN UDP
		pin := strings.ReplaceAll(target, "-", "")
		m.log.Info("Searching LAN for pairing host with PIN %s...", pin)
		discoveredEP, discoveredTok, err := m.discoverHostByPIN(ctx, pin)
		if err != nil {
			return nil, fmt.Errorf("failed to locate device with PIN %s: %w", pin, err)
		}
		endpoint = discoveredEP
		token = discoveredTok
	}

	if endpoint == "" {
		return nil, fmt.Errorf("invalid pairing target or endpoint")
	}

	m.log.Info("Connecting to pairing host at %s...", endpoint)

	// Send our credentials to host
	myPayload := PairingPayload{
		DeviceID:   cfg.DeviceID,
		DeviceName: cfg.DeviceName,
		PublicKey:  cfg.PublicKey,
		VirtualIP:  cfg.VirtualIP,
		Endpoint:   fmt.Sprintf("%s:%d", getLocalIP(), cfg.SFTPPort),
		Token:      token,
	}

	payloadBytes, _ := json.Marshal(myPayload)
	reqURL := fmt.Sprintf("http://%s/pair", endpoint)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pairing handshake failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("host rejected pairing with status %d", resp.StatusCode)
	}

	var hostCredentials PairingPayload
	if err := json.NewDecoder(resp.Body).Decode(&hostCredentials); err != nil {
		return nil, fmt.Errorf("invalid response from host: %w", err)
	}

	if expectedName == "" {
		expectedName = hostCredentials.DeviceName
	}

	newPeer := config.Peer{
		DeviceID:         hostCredentials.DeviceID,
		DeviceName:       hostCredentials.DeviceName,
		PublicKey:        hostCredentials.PublicKey,
		VirtualIP:        hostCredentials.VirtualIP,
		LastSeenEndpoint: hostCredentials.Endpoint,
		LastSeenTime:     time.Now(),
		Allowed:          true,
		ACL: config.PeerACL{
			AllowRead:    true,
			AllowWrite:   true,
			BlockedPaths: []string{".ssh", ".git", ".env", "credentials"},
		},
	}

	_ = cfg.AddPeer(newPeer)
	_ = cfg.Save("")

	m.log.Info("Successfully paired with '%s' (%s)", newPeer.DeviceName, newPeer.DeviceID)
	return &newPeer, nil
}

func (m *Manager) runBroadcastResponder(session *PairingSession, pin, token string, port int) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: 7756})
	if err != nil {
		return
	}
	defer conn.Close()

	buf := make([]byte, 1024)
	for {
		if time.Now().After(session.ExpiresAt) {
			return
		}
		_ = conn.SetReadDeadline(time.Now().Add(1 * time.Second))
		n, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			continue
		}

		reqStr := string(buf[:n])
		if strings.HasPrefix(reqStr, "ISTHMUS_PAIR_QUERY:") {
			queryPIN := strings.TrimPrefix(reqStr, "ISTHMUS_PAIR_QUERY:")
			if queryPIN == pin {
				respStr := fmt.Sprintf("ISTHMUS_PAIR_RESP:%d:%s", port, token)
				_, _ = conn.WriteToUDP([]byte(respStr), remoteAddr)
			}
		}
	}
}

func (m *Manager) discoverHostByPIN(ctx context.Context, pin string) (string, string, error) {
	conn, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return "", "", err
	}
	defer conn.Close()

	bcastAddr, err := net.ResolveUDPAddr("udp4", "255.255.255.255:7756")
	if err != nil {
		return "", "", err
	}

	query := []byte(fmt.Sprintf("ISTHMUS_PAIR_QUERY:%s", pin))
	buf := make([]byte, 1024)

	deadline := time.Now().Add(15 * time.Second)
	ticker := time.NewTicker(800 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", "", ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return "", "", fmt.Errorf("timeout waiting for device with PIN %s", pin)
			}
			_, _ = conn.WriteToUDP(query, bcastAddr)

			_ = conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
			n, remoteAddr, err := conn.ReadFromUDP(buf)
			if err == nil && n > 0 {
				respStr := string(buf[:n])
				if strings.HasPrefix(respStr, "ISTHMUS_PAIR_RESP:") {
					parts := strings.Split(strings.TrimPrefix(respStr, "ISTHMUS_PAIR_RESP:"), ":")
					if len(parts) >= 2 {
						port := parts[0]
						token := parts[1]
						endpoint := fmt.Sprintf("%s:%s", remoteAddr.IP.String(), port)
						return endpoint, token, nil
					}
				}
			}
		}
	}
}

func parseQuery(urlStr string) map[string]string {
	res := make(map[string]string)
	idx := strings.Index(urlStr, "?")
	if idx == -1 {
		return res
	}
	query := urlStr[idx+1:]
	for _, pair := range strings.Split(query, "&") {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) == 2 {
			res[parts[0]] = parts[1]
		}
	}
	return res
}

func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			return ipnet.IP.String()
		}
	}
	return "127.0.0.1"
}
