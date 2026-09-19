package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"dossier/internal/core"
	"dossier/internal/store"
)

func TestDossierTeamToolReturnsRoster(t *testing.T) {
	fake := store.NewFakeStore()
	fake.Roster = &core.Roster{Manager: "hgill", Members: map[string]string{"hgill": "Herwin Gill", "psmith": "Priya Shah"}}
	svc := core.NewService(fake, &mockSearcher{}, &mockTokenizer{}, &mockHarnessRegistry{}, &mockClock{}, core.Config{}, nil)
	var output bytes.Buffer
	server := NewServer(svc, strings.NewReader(`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"dossier_team","arguments":{}},"id":1}`+"\n"), &output)
	if err := server.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	var response JSONRPCResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	var toolResult struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(response.Result, &toolResult); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Data struct {
			Manager string `json:"manager"`
			Members []struct {
				Username    string `json:"username"`
				DisplayName string `json:"display_name"`
			} `json:"members"`
		} `json:"data"`
	}
	if len(toolResult.Content) != 1 || json.Unmarshal([]byte(toolResult.Content[0].Text), &result) != nil {
		t.Fatalf("invalid team tool response: %+v", toolResult)
	}
	if result.Data.Manager != "hgill" || len(result.Data.Members) != 2 || result.Data.Members[0].Username != "hgill" || result.Data.Members[1].DisplayName != "Priya Shah" {
		t.Fatalf("team response = %+v", result.Data)
	}
}
