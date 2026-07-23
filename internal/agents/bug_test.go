package agents

import (
	"testing"

	"go.uber.org/zap"
)

func TestNewBugDetectionAgent(t *testing.T) {
	agent := NewBugDetectionAgent(nil, zap.NewNop())
	if agent.Name() != AgentBug {
		t.Fatalf("name=%s want %s", agent.Name(), AgentBug)
	}
}

func TestNewDefaultAgentsIncludesBug(t *testing.T) {
	agents := NewDefaultAgents(nil, nil, zap.NewNop())
	found := false
	for _, a := range agents {
		if a.Name() == AgentBug {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("NewDefaultAgents should include Bug Detection Agent")
	}
	if len(agents) < 7 {
		t.Fatalf("expected at least 7 specialists, got %d", len(agents))
	}
}
