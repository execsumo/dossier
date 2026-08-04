package mcp

import (
	"context"
	"dossier/internal/core"
	"dossier/internal/harness"
	"encoding/json"
	"fmt"
)

// ToolDefinition represents an MCP tool definition.
type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type mcpEnvelope struct {
	OK          bool            `json:"ok"`
	Data        any             `json:"data,omitempty"`
	Error       *mcpErrorObject `json:"error,omitempty"`
	Warnings    []string        `json:"warnings"`
	NextActions []string        `json:"next_actions"`
}

type mcpErrorObject struct {
	Code    MCPErrorCode   `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

func getToolDefinitions() []ToolDefinition {
	return []ToolDefinition{
		{
			Name:        "dossier_list",
			Description: "List open dossiers sorted by priority score",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"status": map[string]any{
						"type":        "string",
						"description": "Filter by status (active|waiting|blocked|resolved|archived|all)",
					},
				},
			},
		},
		{
			Name:        "dossier_recall",
			Description: "Recall a dossier's details and distilled state",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{
						"type":        "string",
						"description": "The slug or ID of the dossier to recall",
					},
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "dossier_search",
			Description: "Search distilled state and artifacts across dossiers",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "The search query term",
					},
					"dossier_id": map[string]any{
						"type":        "string",
						"description": "Scope search to a specific dossier (optional)",
					},
				},
				"required": []string{"query"},
			},
		},
		{
			Name:        "dossier_save",
			Description: "Save a dossier's distilled state and/or update its metadata and artifacts",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{
						"type":        "string",
						"description": "The dossier slug or ID to update. Required; use dossier_promote to create a new dossier.",
					},
					"base_revision": map[string]any{
						"type":        "string",
						"description": "The base revision for optimistic locking concurrency checks",
					},
					"distilled_state_markdown": map[string]any{
						"type":        "string",
						"description": "The new distilled state markdown body",
					},
					"frontmatter_updates": map[string]any{
						"type":        "object",
						"description": "Key-value updates to frontmatter fields",
					},
					"artifacts": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"type":           map[string]any{"type": "string"},
								"title":          map[string]any{"type": "string"},
								"content_format": map[string]any{"type": "string"},
								"content":        map[string]any{"type": "string"},
								"provenance": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"origin": map[string]any{"type": "string"},
										"url":    map[string]any{"type": "string"},
									},
								},
							},
							"required": []string{"type", "title", "content_format", "content"},
						},
					},
				},
			},
		},
		{
			Name:        "dossier_promote",
			Description: "Create a new dossier from agent-provided content or file",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":                     map[string]any{"type": "string"},
					"distilled_state_markdown": map[string]any{"type": "string"},
					"from_file_path":           map[string]any{"type": "string"},
				},
				"required": []string{"name", "distilled_state_markdown"},
			},
		},
		{
			Name:        "dossier_link",
			Description: "Link session content or files to an existing dossier",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":             map[string]any{"type": "string"},
					"from_file_path": map[string]any{"type": "string"},
				},
			},
		},
		{
			Name:        "dossier_merge",
			Description: "Merge a source dossier into a target dossier",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"source_id": map[string]any{"type": "string"},
					"target_id": map[string]any{"type": "string"},
				},
				"required": []string{"source_id", "target_id"},
			},
		},
		{
			Name:        "dossier_session",
			Description: "Get the active dossier bound to the current session, or switch/bind the session to a dossier (by slug or id) if the 'id' parameter is provided.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":         map[string]any{"type": "string", "description": "Optional: Dossier slug or id to bind to the active session. If omitted, the current active dossier is returned."},
					"session_id": map[string]any{"type": "string", "description": "Optional override; normally omit. Defaults to the current Claude Code session."},
				},
			},
		},
		{
			Name:        "dossier_update",
			Description: "Update a dossier's metadata fields — name, status, lead assignee, next action, open questions, and/or priority (importance, urgency, due date). All fields except id are optional; only supplied fields are written.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":             map[string]any{"type": "string", "description": "The dossier slug or ID to update"},
					"name":           map[string]any{"type": "string", "description": "Replace the display name (omit to leave unchanged). The slug/directory is the durable identifier and does NOT change on rename."},
					"status":         map[string]any{"type": "string", "description": "Replace the current status: active|waiting|blocked|resolved|archived (omit to leave unchanged)"},
					"lead":           map[string]any{"type": "string", "description": "Replace the lead assignee (omit to leave unchanged)"},
					"next_action":    map[string]any{"type": "string", "description": "Replace the current next action (omit to leave unchanged)"},
					"open_questions": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Replace the open questions list (omit to leave unchanged)"},
					"importance":     map[string]any{"type": "string", "description": "high|low (omit to leave unchanged)"},
					"urgency":        map[string]any{"type": "string", "description": "high|low (omit to leave unchanged)"},
					"due_date":       map[string]any{"type": "string", "description": "ISO 8601 date or empty string to clear (omit to leave unchanged)"},
				},
				"required": []string{"id"},
			},
		},
	}
}

func (s *Server) handleToolCall(ctx context.Context, id any, name string, args json.RawMessage) {
	var err error
	var res core.Result

	switch name {
	case "dossier_list":
		var params struct {
			Status string `json:"status"`
		}
		_ = json.Unmarshal(args, &params)
		res, err = s.svc.List(ctx, core.ListReq{Status: params.Status})

	case "dossier_recall":
		var params struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(args, &params); err != nil {
			s.sendError(id, -32602, "Missing id", nil)
			return
		}
		res, err = s.svc.Recall(ctx, core.RecallReq{ID: params.ID})

	case "dossier_search":
		var params struct {
			Query     string `json:"query"`
			DossierID string `json:"dossier_id"`
		}
		if err := json.Unmarshal(args, &params); err != nil {
			s.sendError(id, -32602, "Missing query", nil)
			return
		}
		res, err = s.svc.Search(ctx, core.SearchReq{
			Query: params.Query,
			Scope: core.SearchScope{DossierID: params.DossierID},
		})

	case "dossier_save":
		var params struct {
			ID                     string         `json:"id"`
			BaseRevision           core.Revision  `json:"base_revision"`
			DistilledStateMarkdown string         `json:"distilled_state_markdown"`
			FrontmatterUpdates     map[string]any `json:"frontmatter_updates"`
			Artifacts              []struct {
				Type          core.ArtifactType  `json:"type"`
				Title         string             `json:"title"`
				ContentFormat core.ContentFormat `json:"content_format"`
				Content       string             `json:"content"`
				Provenance    *struct {
					Origin string `json:"origin"`
					URL    string `json:"url"`
				} `json:"provenance"`
			} `json:"artifacts"`
		}

		if err := json.Unmarshal(args, &params); err != nil {
			s.sendError(id, -32602, "Invalid params", nil)
			return
		}

		var arts []core.Artifact
		for _, a := range params.Artifacts {
			artItem := core.Artifact{
				Type:          a.Type,
				Title:         a.Title,
				ContentFormat: a.ContentFormat,
				Content:       a.Content,
			}
			if a.Provenance != nil {
				artItem.Provenance = core.Provenance{
					Origin: a.Provenance.Origin,
					URL:    a.Provenance.URL,
				}
			}
			arts = append(arts, artItem)
		}

		res, err = s.svc.Save(ctx, core.SaveReq{
			ID:                     params.ID,
			BaseRevision:           params.BaseRevision,
			DistilledStateMarkdown: params.DistilledStateMarkdown,
			FrontmatterUpdates:     params.FrontmatterUpdates,
			Artifacts:              arts,
		})

	case "dossier_promote":
		var params struct {
			Name                   string `json:"name"`
			DistilledStateMarkdown string `json:"distilled_state_markdown"`
			FromFilePath           string `json:"from_file_path"`
			SessionContent         string `json:"session_content"`
			Force                  bool   `json:"force"`
		}
		if err := json.Unmarshal(args, &params); err != nil {
			s.sendError(id, -32602, "Invalid params", nil)
			return
		}
		res, err = s.svc.Promote(ctx, core.PromoteReq{
			Name:                   params.Name,
			DistilledStateMarkdown: params.DistilledStateMarkdown,
			FromFilePath:           params.FromFilePath,
			Content:                params.SessionContent,
			Force:                  params.Force,
		})

	case "dossier_link":
		var params struct {
			ID             string `json:"id"`
			FromFilePath   string `json:"from_file_path"`
			SessionContent string `json:"session_content"`
		}
		if err := json.Unmarshal(args, &params); err != nil {
			s.sendError(id, -32602, "Invalid params", nil)
			return
		}
		res, err = s.svc.Link(ctx, core.LinkReq{
			ID:           params.ID,
			FromFilePath: params.FromFilePath,
			Content:      params.SessionContent,
		})

	case "dossier_merge":
		var params struct {
			SourceID          string   `json:"source_id"`
			TargetID          string   `json:"target_id"`
			ResolvedConflicts []string `json:"resolved_conflicts"`
		}
		if err := json.Unmarshal(args, &params); err != nil {
			s.sendError(id, -32602, "Invalid params", nil)
			return
		}
		res, err = s.svc.Merge(ctx, core.MergeReq{
			SourceID:          params.SourceID,
			TargetID:          params.TargetID,
			ResolvedConflicts: params.ResolvedConflicts,
		})

	case "dossier_session":
		var params struct {
			ID        string `json:"id"`
			SessionID string `json:"session_id"`
		}
		_ = json.Unmarshal(args, &params)
		sid, sourceHarness, serr := harness.ResolveSession(params.SessionID, false)
		if serr != nil {
			err = core.NewError(core.ErrHarnessCapabilityUnavailable, serr.Error())
		} else {
			if params.ID != "" {
				res, err = s.svc.Switch(ctx, core.SwitchReq{ID: params.ID, SessionID: sid, HarnessName: sourceHarness})
			} else {
				res, err = s.svc.Active(ctx, core.ActiveReq{SessionID: sid})
			}
			if err == nil && res.OK {
				type SessionResponse struct {
					State                 interface{} `json:"state"`
					Guide                 string      `json:"distillation_guide,omitempty"`
					OperatingInstructions string      `json:"operating_instructions,omitempty"`
				}
				res.Data = SessionResponse{
					State:                 res.Data,
					Guide:                 s.svc.GetGuide(),
					OperatingInstructions: s.svc.GetInstructions(),
				}
			}
		}

	case "dossier_update":
		var params struct {
			ID            string   `json:"id"`
			Name          *string  `json:"name"`
			Status        *string  `json:"status"`
			Lead          *string  `json:"lead"`
			NextAction    *string  `json:"next_action"`
			OpenQuestions []string `json:"open_questions"`
			Importance    string   `json:"importance"`
			Urgency       string   `json:"urgency"`
			DueDate       string   `json:"due_date"`
		}
		if err := json.Unmarshal(args, &params); err != nil {
			s.sendError(id, -32602, "Invalid params", nil)
			return
		}
		updates := map[string]any{}
		if params.Name != nil {
			updates["name"] = *params.Name
		}
		if params.Status != nil {
			updates["status"] = *params.Status
		}
		if params.Lead != nil {
			updates["lead"] = *params.Lead
		}
		if params.NextAction != nil {
			updates["next_action"] = *params.NextAction
		}
		if params.OpenQuestions != nil {
			updates["open_questions"] = params.OpenQuestions
		}
		if params.Importance != "" {
			updates["importance"] = params.Importance
		}
		if params.Urgency != "" {
			updates["urgency"] = params.Urgency
		}
		if params.DueDate != "" {
			updates["due_date"] = params.DueDate
		}
		res, err = s.svc.Save(ctx, core.SaveReq{
			ID:                 params.ID,
			FrontmatterUpdates: updates,
		})

	default:
		s.sendError(id, -32601, fmt.Sprintf("Tool %s not found", name), nil)
		return
	}

	var env mcpEnvelope
	if err != nil {
		code, msg := MapError(err)
		env.OK = false
		env.Error = &mcpErrorObject{
			Code:    code,
			Message: msg,
		}
		// For ambiguous_target, include candidate data and next_actions so the agent
		// can present the disambiguation to the user without an extra round-trip.
		if code == ErrCodeAmbiguousTarget {
			env.Data = res.Data
			for _, na := range res.NextActions {
				env.NextActions = append(env.NextActions, string(na))
			}
		}
	} else {
		env.OK = res.OK
		env.Data = res.Data
		for _, w := range res.Warnings {
			env.Warnings = append(env.Warnings, string(w))
		}
		for _, na := range res.NextActions {
			env.NextActions = append(env.NextActions, string(na))
		}
	}

	envBytes, marshalErr := json.Marshal(env)
	if marshalErr != nil {
		s.sendError(id, -32603, "Failed to marshal envelope", nil)
		return
	}

	type contentItem struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	type toolCallResult struct {
		Content []contentItem `json:"content"`
	}

	result := toolCallResult{
		Content: []contentItem{
			{
				Type: "text",
				Text: string(envBytes),
			},
		},
	}

	s.sendResult(id, result)
}
