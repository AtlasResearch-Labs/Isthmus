package coord

import (
	"time"
)

type MsgType string

const (
	MsgRegister     MsgType = "register"
	MsgRegisterAck  MsgType = "register_ack"
	MsgHeartbeat    MsgType = "heartbeat"
	MsgHeartbeatAck MsgType = "heartbeat_ack"
	MsgPeerExchange MsgType = "peer_exchange"
	MsgPeerUpdate   MsgType = "peer_update"
	MsgRelayPacket  MsgType = "relay_packet"
	MsgSTUNRequest  MsgType = "stun_request"
	MsgSTUNResponse MsgType = "stun_response"
)

type RegisterRequest struct {
	Type       MsgType `json:"type"`
	DeviceID   string  `json:"device_id"`
	DeviceName string  `json:"device_name"`
	PublicKey  string  `json:"public_key"`
	VirtualIP  string  `json:"virtual_ip"`
	ListenPort int     `json:"listen_port"`
	SFTPPort   int     `json:"sftp_port"`
	Timestamp  int64   `json:"timestamp"`
	AuthToken  string  `json:"auth_token,omitempty"`
}

type RegisterResponse struct {
	Type          MsgType `json:"type"`
	Success       bool    `json:"success"`
	AssignedIP    string  `json:"assigned_ip,omitempty"`
	ReflectedAddr string  `json:"reflected_addr,omitempty"`
	Error         string  `json:"error,omitempty"`
	Timestamp     int64   `json:"timestamp"`
}

type HeartbeatRequest struct {
	Type      MsgType `json:"type"`
	DeviceID  string  `json:"device_id"`
	Timestamp int64   `json:"timestamp"`
	AuthToken string  `json:"auth_token,omitempty"`
}

type HeartbeatResponse struct {
	Type          MsgType `json:"type"`
	Success       bool    `json:"success"`
	ReflectedAddr string  `json:"reflected_addr"`
	Timestamp     int64   `json:"timestamp"`
	Error         string  `json:"error,omitempty"`
}

type STUNResponse struct {
	Type          MsgType `json:"type"`
	ReflectedIP   string  `json:"reflected_ip"`
	ReflectedPort int     `json:"reflected_port"`
	ReflectedAddr string  `json:"reflected_addr"`
	Timestamp     int64   `json:"timestamp"`
}

type PeerExchangeRequest struct {
	Type         MsgType `json:"type"`
	SourceDevice string  `json:"source_device"`
	TargetDevice string  `json:"target_device"`
	Timestamp    int64   `json:"timestamp"`
	AuthToken    string  `json:"auth_token,omitempty"`
}

type PeerExchangeResponse struct {
	Type          MsgType   `json:"type"`
	TargetDevice  string    `json:"target_device"`
	TargetName    string    `json:"target_name"`
	PublicKey     string    `json:"public_key"`
	VirtualIP     string    `json:"virtual_ip"`
	PublicAddr    string    `json:"public_addr"`
	SFTPPort      int       `json:"sftp_port"`
	TunnelPort    int       `json:"tunnel_port"`
	RelayEndpoint string    `json:"relay_endpoint,omitempty"`
	RelayEnabled  bool      `json:"relay_enabled"`
	LastSeen      time.Time `json:"last_seen"`
	Error         string    `json:"error,omitempty"`
}
