package updater

import (
	"testing"
)

func TestCheckUpdate(t *testing.T) {
	info, err := CheckUpdate(true)
	if err != nil {
		t.Fatalf("CheckUpdate failed: %v", err)
	}
	if info == nil {
		t.Fatal("expected non-nil UpdateInfo")
	}
	t.Logf("Update check result: Current=%s, Latest=%s, HasUpdate=%v", info.CurrentVersion, info.LatestVersion, info.HasUpdate)
}
