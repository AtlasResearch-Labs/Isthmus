package fileserver

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"isthmus/internal/logger"
)

type ClientConfig struct {
	Endpoint string
	Password string
	Timeout  time.Duration
}

type ProgressCallback func(bytesTransferred, totalBytes int64, speedBytesPerSec float64)

type Client struct {
	config     ClientConfig
	sshConn    *ssh.Client
	sftpClient *sftp.Client
	log        *logger.Logger
}

func NewClient(cfg ClientConfig) (*Client, error) {
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Second
	}

	conn, err := net.DialTimeout("tcp", cfg.Endpoint, cfg.Timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", cfg.Endpoint, err)
	}

	return NewClientFromConn(conn, cfg)
}

func NewClientFromConn(conn net.Conn, cfg ClientConfig) (*Client, error) {
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Second
	}

	sshConfig := &ssh.ClientConfig{
		User:            "isthmus",
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         cfg.Timeout,
	}

	if cfg.Password != "" {
		sshConfig.Auth = []ssh.AuthMethod{
			ssh.Password(cfg.Password),
		}
	} else {
		sshConfig.Auth = []ssh.AuthMethod{
			ssh.Password(""),
		}
	}

	addrStr := cfg.Endpoint
	if addrStr == "" {
		addrStr = conn.RemoteAddr().String()
	}

	sConn, chans, reqs, err := ssh.NewClientConn(conn, addrStr, sshConfig)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("SSH handshake failed with %s: %w", addrStr, err)
	}

	sshClient := ssh.NewClient(sConn, chans, reqs)
	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		sshClient.Close()
		return nil, fmt.Errorf("failed to initialize SFTP client session: %w", err)
	}

	return &Client{
		config:     cfg,
		sshConn:    sshClient,
		sftpClient: sftpClient,
		log:        logger.WithPrefix("SFTP-Client"),
	}, nil
}

func (c *Client) List(remotePath string) ([]os.FileInfo, error) {
	if remotePath == "" {
		remotePath = "."
	}
	return c.sftpClient.ReadDir(remotePath)
}

func (c *Client) Stat(remotePath string) (os.FileInfo, error) {
	if remotePath == "" {
		remotePath = "."
	}
	return c.sftpClient.Stat(remotePath)
}

func (c *Client) MkdirAll(remoteDir string) error {
	remoteDir = strings.ReplaceAll(remoteDir, "\\", "/")
	parts := strings.Split(remoteDir, "/")
	current := ""
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		if current == "" {
			current = part
		} else {
			current = current + "/" + part
		}
		_ = c.sftpClient.Mkdir(current)
	}
	return nil
}

func (c *Client) Walk(remoteRoot string, walkFn func(path string, info os.FileInfo, err error) error) error {
	if remoteRoot == "" {
		remoteRoot = "."
	}
	walker := c.sftpClient.Walk(remoteRoot)
	for walker.Step() {
		if walker.Err() != nil {
			if err := walkFn(walker.Path(), nil, walker.Err()); err != nil {
				return err
			}
			continue
		}
		if err := walkFn(walker.Path(), walker.Stat(), nil); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) PullFile(remotePath, localPath string, onProgress ProgressCallback) (string, error) {
	return c.pullFileInternal(remotePath, localPath, false, onProgress)
}

func (c *Client) PullFileResume(remotePath, localPath string, onProgress ProgressCallback) (string, error) {
	return c.pullFileInternal(remotePath, localPath, true, onProgress)
}

func (c *Client) pullFileInternal(remotePath, localPath string, allowResume bool, onProgress ProgressCallback) (string, error) {
	remoteFile, err := c.sftpClient.Open(remotePath)
	if err != nil {
		return "", fmt.Errorf("failed to open remote file %s: %w", remotePath, err)
	}
	defer remoteFile.Close()

	stat, err := remoteFile.Stat()
	if err != nil {
		return "", fmt.Errorf("failed to stat remote file: %w", err)
	}
	totalSize := stat.Size()

	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return "", fmt.Errorf("failed to create local directory: %w", err)
	}

	var localFile *os.File
	var existingOffset int64

	if allowResume {
		if localStat, err := os.Stat(localPath); err == nil {
			if localStat.Size() < totalSize {
				existingOffset = localStat.Size()
				localFile, err = os.OpenFile(localPath, os.O_WRONLY|os.O_APPEND, 0644)
				if err != nil {
					localFile = nil
					existingOffset = 0
				} else {
					if _, err := remoteFile.Seek(existingOffset, io.SeekStart); err != nil {
						localFile.Close()
						localFile = nil
						existingOffset = 0
					}
				}
			}
		}
	}

	if localFile == nil {
		localFile, err = os.OpenFile(localPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			return "", fmt.Errorf("failed to create local file %s: %w", localPath, err)
		}
		existingOffset = 0
	}
	defer localFile.Close()

	buf := make([]byte, 128*1024)
	transferred := existingOffset
	startTime := time.Now()

	for {
		n, readErr := remoteFile.Read(buf)
		if n > 0 {
			written, writeErr := localFile.Write(buf[:n])
			if writeErr != nil {
				return "", fmt.Errorf("write error: %w", writeErr)
			}
			transferred += int64(written)

			if onProgress != nil {
				elapsed := time.Since(startTime).Seconds()
				var speed float64
				if elapsed > 0 {
					speed = float64(transferred-existingOffset) / elapsed
				}
				onProgress(transferred, totalSize, speed)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", fmt.Errorf("read error: %w", readErr)
		}
	}

	localFile.Close()

	// Compute full file checksum
	checksum, err := computeFileSHA256(localPath)
	if err != nil {
		return "", fmt.Errorf("checksum calculation error: %w", err)
	}

	return checksum, nil
}

func computeFileSHA256(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func (c *Client) PushFile(localPath, remotePath string, onProgress ProgressCallback) error {
	localFile, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open local file %s: %w", localPath, err)
	}
	defer localFile.Close()

	stat, err := localFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat local file: %w", err)
	}
	totalSize := stat.Size()

	remoteFile, err := c.sftpClient.Create(remotePath)
	if err != nil {
		return fmt.Errorf("failed to create remote file %s: %w", remotePath, err)
	}
	defer remoteFile.Close()

	buf := make([]byte, 128*1024)
	var transferred int64
	startTime := time.Now()

	for {
		n, readErr := localFile.Read(buf)
		if n > 0 {
			written, writeErr := remoteFile.Write(buf[:n])
			if writeErr != nil {
				return fmt.Errorf("remote write error: %w", writeErr)
			}
			transferred += int64(written)

			if onProgress != nil {
				elapsed := time.Since(startTime).Seconds()
				var speed float64
				if elapsed > 0 {
					speed = float64(transferred) / elapsed
				}
				onProgress(transferred, totalSize, speed)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return fmt.Errorf("local read error: %w", readErr)
		}
	}

	return nil
}

func (c *Client) Close() error {
	if c.sftpClient != nil {
		c.sftpClient.Close()
	}
	if c.sshConn != nil {
		return c.sshConn.Close()
	}
	return nil
}
