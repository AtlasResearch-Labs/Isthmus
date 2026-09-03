package service

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	ServiceName = "isthmus"
	DisplayName = "Isthmus Secure Mesh Agent"
	Description = "Cross-device secure tunnel and file access system daemon"
)

type Manager struct {
	ServiceName string
	DisplayName string
	Description string
}

func NewManager() *Manager {
	return &Manager{
		ServiceName: ServiceName,
		DisplayName: DisplayName,
		Description: Description,
	}
}

func (m *Manager) Install(binPath, sharedDir string) error {
	if binPath == "" {
		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("failed to determine executable path: %w", err)
		}
		binPath = exe
	}

	absBin, err := filepath.Abs(binPath)
	if err != nil {
		return err
	}

	if runtime.GOOS == "windows" {
		binArg := fmt.Sprintf("\"%s\" daemon", absBin)
		cmd := exec.Command("sc.exe", "create", m.ServiceName,
			"binPath=", binArg,
			"start=", "auto",
			"DisplayName=", m.DisplayName,
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("sc create failed: %s (%w)", string(out), err)
		}
		return nil
	} else if runtime.GOOS == "linux" {
		var userDirectives string
		sudoUser := os.Getenv("SUDO_USER")
		if sudoUser != "" && sudoUser != "root" {
			homeDir := "/home/" + sudoUser
			group := sudoUser
			if u, err := user.Lookup(sudoUser); err == nil {
				if u.HomeDir != "" {
					homeDir = u.HomeDir
				}
				if g, err := user.LookupGroupId(u.Gid); err == nil && g.Name != "" {
					group = g.Name
				}
			}
			userDirectives = fmt.Sprintf("User=%s\nGroup=%s\nEnvironment=HOME=%s\nEnvironment=XDG_CONFIG_HOME=%s/.config\n",
				sudoUser, group, homeDir, homeDir)
		}

		unitContent := fmt.Sprintf(`[Unit]
Description=%s
After=network.target network-online.target
Wants=network-online.target

[Service]
Type=simple
%sExecStart=%s daemon
Restart=always
RestartSec=3s
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
`, m.Description, userDirectives, absBin)

		unitPath := "/etc/systemd/system/isthmus.service"
		if err := os.WriteFile(unitPath, []byte(unitContent), 0644); err != nil {
			return fmt.Errorf("failed to write systemd unit file (root permissions required): %w", err)
		}

		_ = exec.Command("systemctl", "daemon-reload").Run()
		_ = exec.Command("systemctl", "enable", m.ServiceName).Run()
		return nil
	}

	return fmt.Errorf("service installation not supported on %s", runtime.GOOS)
}

func (m *Manager) Start() error {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("net", "start", m.ServiceName)
	} else if runtime.GOOS == "linux" {
		cmd = exec.Command("systemctl", "start", m.ServiceName)
	} else {
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to start service: %s (%w)", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (m *Manager) Stop() error {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("net", "stop", m.ServiceName)
	} else if runtime.GOOS == "linux" {
		cmd = exec.Command("systemctl", "stop", m.ServiceName)
	} else {
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to stop service: %s (%w)", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (m *Manager) Status() (string, error) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("sc.exe", "query", m.ServiceName)
	} else if runtime.GOOS == "linux" {
		cmd = exec.Command("systemctl", "status", m.ServiceName)
	} else {
		return "", fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}

	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func (m *Manager) Uninstall() error {
	if runtime.GOOS == "windows" {
		_ = exec.Command("sc.exe", "stop", m.ServiceName).Run()
		cmd := exec.Command("sc.exe", "delete", m.ServiceName)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("sc delete failed: %s (%w)", strings.TrimSpace(string(out)), err)
		}
		return nil
	} else if runtime.GOOS == "linux" {
		_ = exec.Command("systemctl", "stop", m.ServiceName).Run()
		_ = exec.Command("systemctl", "disable", m.ServiceName).Run()
		_ = os.Remove("/etc/systemd/system/isthmus.service")
		_ = exec.Command("systemctl", "daemon-reload").Run()
		return nil
	}
	return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
}
