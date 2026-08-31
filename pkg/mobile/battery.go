package mobile

import (
	"os"
	"strconv"
	"strings"
)

type PowerStatus struct {
	BatteryPercentage int  `json:"battery_percentage"`
	IsCharging        bool `json:"is_charging"`
	LowPowerMode      bool `json:"low_power_mode"`
}

// GetPowerStatus checks the current device power and battery level
func GetPowerStatus() PowerStatus {
	status := PowerStatus{
		BatteryPercentage: 100,
		IsCharging:        true,
		LowPowerMode:      false,
	}

	// 1. Check Linux / Android sysfs power supply
	capData, err := os.ReadFile("/sys/class/power_supply/battery/capacity")
	if err == nil {
		if val, err := strconv.Atoi(strings.TrimSpace(string(capData))); err == nil {
			status.BatteryPercentage = val
		}
	}

	statData, err := os.ReadFile("/sys/class/power_supply/battery/status")
	if err == nil {
		statStr := strings.ToLower(strings.TrimSpace(string(statData)))
		status.IsCharging = (statStr == "charging" || statStr == "full")
	}

	if status.BatteryPercentage <= 20 && !status.IsCharging {
		status.LowPowerMode = true
	}

	return status
}

// ShouldThrottleTransfer returns true if background syncing should be deferred to save battery
func ShouldThrottleTransfer() bool {
	ps := GetPowerStatus()
	return ps.LowPowerMode
}
