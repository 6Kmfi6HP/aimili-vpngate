package vpngate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseAPIFixture(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "vpngate_api_sample.csv"))
	if err != nil {
		t.Fatal(err)
	}
	nodes, err := ParseAPI(string(raw))
	if err != nil {
		t.Fatalf("ParseAPI: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("len(nodes) = %d", len(nodes))
	}
	node := nodes[0]
	if node.ID != "jp-198-51-100-10" {
		t.Fatalf("ID = %q", node.ID)
	}
	if node.RemoteHost != "198.51.100.10" || node.RemotePort != 443 || node.RemoteProto != "tcp" {
		t.Fatalf("remote not parsed: %#v", node)
	}
	if node.ConfigText == "" {
		t.Fatal("ConfigText is empty")
	}
}

func TestSelectCandidatesSkipsMissingRemote(t *testing.T) {
	nodes := []Node{{ID: "a", RemoteHost: "127.0.0.1", RemotePort: 443}, {ID: "b"}}
	got := SelectCandidates(nodes, 10)
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("unexpected candidates: %#v", got)
	}
}
