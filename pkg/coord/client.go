package coord

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"isthmus/internal/logger"
	"isthmus/pkg/config"
)

type Client struct {
	serverURL  string
	authToken  string
	cfg        *config.Config
	httpClient *http.Client
	log        *logger.Logger
}

func NewClient(serverURL, authToken string, cfg *config.Config) *Client {
	serverURL = strings.TrimRight(serverURL, "/")
	if !strings.HasPrefix(serverURL, "http://") && !strings.HasPrefix(serverURL, "https://") {
		serverURL = "http://" + serverURL
	}

	return &Client{
		serverURL: serverURL,
		authToken: authToken,
		cfg:       cfg,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		log: logger.WithPrefix("CoordClient"),
	}
}

func (c *Client) Register(ctx context.Context) (*RegisterResponse, error) {
	reqPayload := RegisterRequest{
		Type:       MsgRegister,
		DeviceID:   c.cfg.DeviceID,
		DeviceName: c.cfg.DeviceName,
		PublicKey:  c.cfg.PublicKey,
		VirtualIP:  c.cfg.VirtualIP,
		ListenPort: c.cfg.ListenPort,
		SFTPPort:   c.cfg.SFTPPort,
		Timestamp:  time.Now().Unix(),
		AuthToken:  c.authToken,
	}

	data, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to encode register request: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/register", c.serverURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("registration request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registration failed with status code %d", resp.StatusCode)
	}

	var regResp RegisterResponse
	if err := json.NewDecoder(resp.Body).Decode(&regResp); err != nil {
		return nil, fmt.Errorf("failed to decode register response: %w", err)
	}

	if !regResp.Success {
		return &regResp, fmt.Errorf("registration rejected: %s", regResp.Error)
	}

	c.log.Info("Registered on coordination server (%s). Reflected WAN: %s", c.serverURL, regResp.ReflectedAddr)
	return &regResp, nil
}

func (c *Client) Heartbeat(ctx context.Context) (*HeartbeatResponse, error) {
	reqPayload := HeartbeatRequest{
		Type:      MsgHeartbeat,
		DeviceID:  c.cfg.DeviceID,
		Timestamp: time.Now().Unix(),
		AuthToken: c.authToken,
	}

	data, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/api/v1/heartbeat", c.serverURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var hbResp HeartbeatResponse
	if err := json.NewDecoder(resp.Body).Decode(&hbResp); err != nil {
		return nil, err
	}

	return &hbResp, nil
}

func (c *Client) STUN(ctx context.Context) (*STUNResponse, error) {
	url := fmt.Sprintf("%s/api/v1/stun", c.serverURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("STUN request failed: %w", err)
	}
	defer resp.Body.Close()

	var stunResp STUNResponse
	if err := json.NewDecoder(resp.Body).Decode(&stunResp); err != nil {
		return nil, err
	}

	return &stunResp, nil
}

func (c *Client) ExchangePeer(ctx context.Context, target string) (*PeerExchangeResponse, error) {
	reqPayload := PeerExchangeRequest{
		Type:         MsgPeerExchange,
		SourceDevice: c.cfg.DeviceID,
		TargetDevice: target,
		Timestamp:    time.Now().Unix(),
		AuthToken:    c.authToken,
	}

	data, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/api/v1/peer-exchange", c.serverURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("peer exchange failed: %w", err)
	}
	defer resp.Body.Close()

	var exResp PeerExchangeResponse
	if err := json.NewDecoder(resp.Body).Decode(&exResp); err != nil {
		return nil, err
	}

	if exResp.Error != "" {
		return &exResp, fmt.Errorf("peer exchange error: %s", exResp.Error)
	}

	return &exResp, nil
}

func (c *Client) StartHeartbeatLoop(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 25 * time.Second
	}

	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				hbCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				_, err := c.Heartbeat(hbCtx)
				cancel()
				if err != nil {
					c.log.Debug("Heartbeat ping failed: %v", err)
				}
			}
		}
	}()
}
