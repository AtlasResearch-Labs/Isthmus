package mobile

import (
	"testing"
)

func TestPowerStatus(t *testing.T) {
	status := GetPowerStatus()
	if status.BatteryPercentage < 0 || status.BatteryPercentage > 100 {
		t.Errorf("invalid battery percentage: %d", status.BatteryPercentage)
	}

	_ = ShouldThrottleTransfer()
}
