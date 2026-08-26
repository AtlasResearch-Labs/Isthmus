package fileserver

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"isthmus/internal/logger"
)

type ServerConfig struct {
	Port         int
	RootDir      string
	HostKeyPEM   []byte
	AuthPassword string
	AllowedKeys  []string
	ReadOnly     bool
}

type Server struct {
	mu         sync.Mutex
	config     ServerConfig
	listener   net.Listener
	sshConfig  *ssh.ServerConfig
	activeConn map[net.Conn]struct{}
	stopChan   chan struct{}
	log        *logger.Logger
}

func generateDefaultHostKey() ([]byte, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	return keyPEM, nil
}

func NewServer(cfg ServerConfig) (*Server, error) {
	if cfg.RootDir == "" {
		cfg.RootDir = "."
	}

	absRoot, err := filepath.Abs(cfg.RootDir)
	if err != nil {
		return nil, fmt.Errorf("invalid root directory: %w", err)
	}
	cfg.RootDir = absRoot

	if err := os.MkdirAll(cfg.RootDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create root directory: %w", err)
	}

	if len(cfg.HostKeyPEM) == 0 {
		hostKey, err := generateDefaultHostKey()
		if err != nil {
			return nil, fmt.Errorf("failed to generate host key: %w", err)
		}
		cfg.HostKeyPEM = hostKey
	}

	signer, err := ssh.ParsePrivateKey(cfg.HostKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to parse host key: %w", err)
	}

	sshConfig := &ssh.ServerConfig{
		NoClientAuth: true,
	}

	if cfg.AuthPassword != "" {
		sshConfig.NoClientAuth = false
		sshConfig.PasswordCallback = func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if string(pass) == cfg.AuthPassword {
				return nil, nil
			}
			return nil, fmt.Errorf("password rejected for %q", c.User())
		}
	}

	sshConfig.AddHostKey(signer)

	return &Server{
		config:     cfg,
		sshConfig:  sshConfig,
		activeConn: make(map[net.Conn]struct{}),
		stopChan:   make(chan struct{}),
		log:        logger.WithPrefix("SFTP-Server"),
	}, nil
}

func (s *Server) Start() error {
	addr := fmt.Sprintf("0.0.0.0:%d", s.config.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	s.mu.Lock()
	s.listener = listener
	s.mu.Unlock()

	s.log.Info("SFTP service listening on %s (Root: %s)", addr, s.config.RootDir)

	go s.acceptLoop()
	return nil
}

func (s *Server) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.stopChan:
				return
			default:
				s.log.Debug("Accept error: %v", err)
				return
			}
		}

		s.mu.Lock()
		s.activeConn[conn] = struct{}{}
		s.mu.Unlock()

		go s.handleConnection(conn)
	}
}

func (s *Server) handleConnection(nConn net.Conn) {
	defer func() {
		nConn.Close()
		s.mu.Lock()
		delete(s.activeConn, nConn)
		s.mu.Unlock()
	}()

	s.log.Info("Incoming connection from %s", nConn.RemoteAddr())

	sConn, chans, reqs, err := ssh.NewServerConn(nConn, s.sshConfig)
	if err != nil {
		s.log.Debug("SSH handshake failed with %s: %v", nConn.RemoteAddr(), err)
		return
	}
	defer sConn.Close()

	go ssh.DiscardRequests(reqs)

	for newChannel := range chans {
		if newChannel.ChannelType() != "session" {
			newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}

		channel, requests, err := newChannel.Accept()
		if err != nil {
			s.log.Debug("Could not accept channel: %v", err)
			continue
		}

		go func(in <-chan *ssh.Request) {
			for req := range in {
				ok := false
				switch req.Type {
				case "subsystem":
					if string(req.Payload[4:]) == "sftp" {
						ok = true
						go s.serveSFTP(channel)
					}
				}
				req.Reply(ok, nil)
			}
		}(requests)
	}
}

func (s *Server) serveSFTP(channel ssh.Channel) {
	defer channel.Close()

	var opts []sftp.ServerOption
	opts = append(opts, sftp.WithDebug(io.Discard), sftp.WithServerWorkingDirectory(s.config.RootDir))
	if s.config.ReadOnly {
		opts = append(opts, sftp.ReadOnly())
	}

	server, err := sftp.NewServer(channel, opts...)
	if err != nil {
		s.log.Error("Failed to initialize SFTP handler: %v", err)
		return
	}

	if err := server.Serve(); err != nil {
		if err != io.EOF {
			s.log.Debug("SFTP server session finished: %v", err)
		}
	}
}

func (s *Server) Port() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener != nil {
		return s.listener.Addr().(*net.TCPAddr).Port
	}
	return s.config.Port
}

func (s *Server) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	select {
	case <-s.stopChan:
		return
	default:
		close(s.stopChan)
	}

	if s.listener != nil {
		s.listener.Close()
	}

	for conn := range s.activeConn {
		conn.Close()
	}
}
