package agents

import (
	"testing"

	"go.uber.org/zap"
)

func TestNewDependencyAgent(t *testing.T) {
	agent := NewDependencyAgent(nil, zap.NewNop())
	if agent.Name() != AgentDependency {
		t.Fatalf("name=%s want %s", agent.Name(), AgentDependency)
	}
}

func TestCollectDependencyManifests(t *testing.T) {
	input := ReviewInput{
		Files: []SourceFile{
			{Path: "main.go", Content: "package main"},
			{Path: "go.mod", Content: "module example\n\nrequire github.com/x/y v1.2.3\n"},
			{Path: "package.json", Content: `{"dependencies":{"lodash":"^4.0.0"}}`},
		},
	}
	got := collectDependencyManifests(input)
	if len(got) != 2 {
		t.Fatalf("expected 2 manifests, got %d (%+v)", len(got), got)
	}
}

func TestNewDefaultAgentsIncludesDependency(t *testing.T) {
	list := NewDefaultAgents(nil, nil, zap.NewNop())
	found := false
	for _, a := range list {
		if a.Name() == AgentDependency {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("NewDefaultAgents should include Dependency Agent")
	}
	if len(list) < 7 {
		t.Fatalf("expected at least 7 specialists, got %d", len(list))
	}
}
