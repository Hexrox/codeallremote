package sdk

import (
	"testing"
)

func TestIsCompatible(t *testing.T) {
	supported := SDKProtocolRange{Min: 1, Max: 1}

	tests := []struct {
		name string
		m    Manifest
		want bool
	}{
		{"exact match", Manifest{SDKRange: SDKProtocolRange{1, 1}}, true},
		{"overlap", Manifest{SDKRange: SDKProtocolRange{1, 2}}, true},
		{"too new", Manifest{SDKRange: SDKProtocolRange{2, 3}}, false},
		{"too old", Manifest{SDKRange: SDKProtocolRange{0, 0}}, false},
		{"no overlap high", Manifest{SDKRange: SDKProtocolRange{5, 7}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.m.PluginID = "p"
			tt.m.Name = "n"
			tt.m.Version = "1.0"
			tt.m.AgentType = "x"
			if got := IsCompatible(tt.m, supported); got != tt.want {
				t.Errorf("IsCompatible = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateManifest(t *testing.T) {
	good := Manifest{
		PluginID: "p", Name: "n", Version: "1.0",
		SDKRange: SDKProtocolRange{1, 1}, AgentType: "claude-code",
	}
	if r := ValidateManifest(good); r != "" {
		t.Errorf("valid manifest rejected: %s", r)
	}

	bad := good
	bad.PluginID = ""
	if r := ValidateManifest(bad); r == "" {
		t.Error("empty plugin id should fail")
	}

	bad = good
	bad.AgentType = ""
	if r := ValidateManifest(bad); r == "" {
		t.Error("empty agent_type should fail")
	}

	bad = good
	bad.SDKRange = SDKProtocolRange{2, 1}
	if r := ValidateManifest(bad); r == "" {
		t.Error("max<min should fail")
	}
}

// stubAdapter is a minimal Adapter impl for contract tests.
type stubAdapter struct {
	manifest Manifest
}

func (s *stubAdapter) ID() string         { return s.manifest.PluginID }
func (s *stubAdapter) Manifest() Manifest { return s.manifest }
func (s *stubAdapter) ValidateWorkspace(string) ValidationResult {
	return ValidationResult{Valid: true}
}
func (s *stubAdapter) Start(StartConfig) (*RunHandle, error)   { return &RunHandle{ID: "r"}, nil }
func (s *stubAdapter) SubmitInput(*RunHandle, string) Accepted { return Accepted{Accepted: true} }
func (s *stubAdapter) Interrupt(*RunHandle) Accepted           { return Accepted{Accepted: true} }
func (s *stubAdapter) Observe(*RunHandle) <-chan Signal {
	ch := make(chan Signal)
	close(ch)
	return ch
}
func (s *stubAdapter) DecideApproval(*RunHandle, string, bool, string) Accepted {
	return Accepted{Accepted: true}
}
func (s *stubAdapter) Recover(string) RecoveryResult { return RecoveryResult{CanRecover: true} }
func (s *stubAdapter) Capabilities() []string        { return s.manifest.Capabilities }
func (s *stubAdapter) SelfCheck() error              { return nil }
func (s *stubAdapter) Drain() error                  { return nil }

// TestAdapter_contract verifies a stub satisfies the Adapter interface.
func TestAdapter_contract(t *testing.T) {
	var _ Adapter = &stubAdapter{manifest: Manifest{PluginID: "stub"}}
}
