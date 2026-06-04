package diagnostics

import (
	"testing"

	"github.com/6Kmfi6HP/aimili-vpngate/internal/config"
)

func TestRuntimeChecksIncludeWritableDataDir(t *testing.T) {
	cfg := config.Load("test")
	cfg.DataDir = t.TempDir()
	checks := RuntimeChecks(cfg)
	var found bool
	for _, check := range checks {
		if check.Name == "data-dir" {
			found = true
			if !check.OK {
				t.Fatalf("data-dir check failed: %#v", check)
			}
		}
	}
	if !found {
		t.Fatal("data-dir check not present")
	}
}
