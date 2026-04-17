package lab

import "testing"

func TestIsCritical(t *testing.T) {
	if !FlagCriticalHigh.IsCritical() {
		t.Fatal("critical_high must be critical")
	}
	if !FlagCriticalLow.IsCritical() {
		t.Fatal("critical_low must be critical")
	}
	if FlagNormal.IsCritical() {
		t.Fatal("normal is not critical")
	}
	if FlagHigh.IsCritical() {
		t.Fatal("high alone is not critical")
	}
}
