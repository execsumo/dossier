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

func getToolDefinitions(configured ...[]string) []ToolDefinition {
	interfaces := core.DefaultDiscussionInterfaces()
	var leads []string
	if len(configured) > 0 {
		interfaces = configured[0]
	}
	if len(configured) > 1 {
		leads = configured[1]
	}
	return []ToolDefinition{
		{
			Name:        "dossier_list",
			Description: "List open dossiers sorted by priority (max, high, medium, low)",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"status": map[string]any{
						"type":        "string",
						"description": "Filter by status (spark|define|delegated|review|blocked|done|all)",
					},
					"interfaces": configuredStringListSchema(interfaces, "Filter by discussion interface; matches dossiers assigned to any supplied interface"),
					"query": map[string]any{
						"type":        "string",
						"description": "Filter by name, description, lead, interface, or slug; whitespace-separated terms are ANDed",
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
			Name:        "dossier_artifact",
			Description: "Fetch archived artifact content by id, optionally a cited line range. Resolves a [src:art_x#L10-L20] citation back to its verbatim source.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"dossier_id": map[string]any{
						"type":        "string",
						"description": "The dossier slug or ID holding the artifact",
					},
					"artifact_id": map[string]any{
						"type":        "string",
						"description": "The artifact ID, as it appears inside a [src:...] citation",
					},
					"fragment": map[string]any{
						"type":        "string",
						"description": "Optional citation fragment to resolve, e.g. \"L10-L20\". Overrides start_line/end_line.",
					},
					"start_line": map[string]any{
						"type":        "integer",
						"description": "Optional 1-indexed first line to return",
					},
					"end_line": map[string]any{
						"type":        "integer",
						"description": "Optional 1-indexed last line to return",
					},
				},
				"required": []string{"dossier_id", "artifact_id"},
			},
		},
		{
			Name:        "dossier_artifacts",
			Description: "List a dossier's evidence index: every archived artifact with its type, line count, and whether the distilled state cites it",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"dossier_id": map[string]any{
						"type":        "string",
						"description": "The dossier slug or ID to index",
					},
				},
				"required": []string{"dossier_id"},
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
						"description": "Key-value updates to frontmatter fields (description and priority: low|medium|high|max)",
						"properties": map[string]any{
							"description": map[string]any{"type": "string"},
							"priority":    map[string]any{"type": "string", "enum": []string{"low", "medium", "high", "max"}},
						},
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
			Description: "Create a new dossier from agent-provided content or file; description is an optional progressive-disclosure summary",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":                     map[string]any{"type": "string"},
					"distilled_state_markdown": map[string]any{"type": "string"},
					"description":              map[string]any{"type": "string", "description": "Optional short progressive-disclosure summary"},
					"priority":                 map[string]any{"type": "string", "enum": []string{"low", "medium", "high", "max"}},
					"from_file_path":           map[string]any{"type": "string"},
					"lead":                     configuredLeadSchema(leads, "Lead assignee; available values come from config.yaml"),
					"interfaces":               configuredStringListSchema(interfaces, "Discussion interfaces; available values come from config.yaml"),
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
					"id":            map[string]any{"type": "string", "description": "Optional: Dossier slug or id to bind to the active session. If omitted, the current active dossier is returned."},
					"session_id":    map[string]any{"type": "string", "description": "Optional override; normally omit. Defaults to the current Claude Code session."},
					"include_guide": map[string]any{"type": "boolean", "description": "Force the Distillation Guide into the response. The Guide is normally sent once per session; set this when it has left your context and you are about to write, rather than saving without it."},
				},
			},
		},
		{
			Name:        "dossier_update",
			Description: "Update a dossier's metadata fields — name, description, status, lead assignee, next action, priority, due date, and interfaces. All fields except id are optional; only supplied fields are written. Use dossier_rename to change the slug.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":          map[string]any{"type": "string", "description": "The dossier slug or ID to update"},
					"name":        map[string]any{"type": "string", "description": "Replace the display name (omit to leave unchanged). The slug does not change; use dossier_rename for that."},
					"description": map[string]any{"type": "string", "description": "Replace the optional progressive-disclosure summary (omit to leave unchanged)"},
					"status":      map[string]any{"type": "string", "description": "Replace the current status: spark|define|delegated|review|blocked|done (omit to leave unchanged)"},
					"lead":        configuredLeadSchema(leads, "Replace the lead assignee (omit to leave unchanged; empty clears)"),
					"next_action": map[string]any{"type": "string", "description": "Replace the current next action (omit to leave unchanged)"},
					"priority":    map[string]any{"type": "string", "enum": []string{"low", "medium", "high", "max"}, "description": "low|medium|high|max (omit to leave unchanged)"},
					"due_date":    map[string]any{"type": "string", "description": "ISO 8601 date or empty string to clear (omit to leave unchanged)"},
					"interfaces":  configuredStringListSchema(interfaces, "Replace the discussion interface list (omit to leave unchanged)"),
				},
				"required": []string{"id"},
			},
		},
		{
			Name:        "dossier_rename",
			Description: "Rename a dossier's canonical slug and move its complete directory. The immutable ID stays fixed and the old slug remains a working alias.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":            map[string]any{"type": "string", "description": "Current dossier slug, historical slug alias, or immutable ID"},
					"new_slug":      map[string]any{"type": "string", "description": "New lowercase ASCII slug using letters, digits, and single hyphens"},
					"base_revision": map[string]any{"type": "string", "description": "Revision returned by dossier_recall; prevents a stale rename"},
				},
				"required": []string{"id", "new_slug", "base_revision"},
			},
		},
	}
}

func configuredStringListSchema(values []string, description string) map[string]any {
	schema := map[string]any{
		"type":        "array",
		"description": description,
	}
	if len(values) == 0 {
		schema["maxItems"] = 0
		return schema
	}
	schema["items"] = map[string]any{"type": "string", "enum": append([]string{}, values...)}
	return schema
}

func configuredLeadSchema(values []string, description string) map[string]any {
	schema := map[string]any{"type": "string", "description": description}
	if len(values) > 0 {
		schema["enum"] = append([]string{""}, values...)
	}
	return schema
}

func (s *Server) handleToolCall(ctx context.Context, id any, name string, args json.RawMessage) {
	var err error
	var res core.Result

	switch name {
	case "dossier_list":
		var params struct {
			Status     string   `json:"status"`
			Interfaces []string `json:"interfaces"`
			Query      string   `json:"query"`
		}
		_ = json.Unmarshal(args, &params)
		res, err = s.svc.List(ctx, core.ListReq{Status: params.Status, Interfaces: params.Interfaces, Query: params.Query})

	case "dossier_recall":
		var params struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(args, &params); err != nil {
			s.sendError(id, -32602, "Missing id", nil)
			return
		}
		res, err = s.svc.Recall(ctx, core.RecallReq{ID: params.ID})
		if err == nil {
			s.triggerSync()
		}

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

	case "dossier_artifact":
		var params struct {
			DossierID  string `json:"dossier_id"`
			ArtifactID string `json:"artifact_id"`
			Fragment   string `json:"fragment"`
			StartLine  int    `json:"start_line"`
			EndLine    int    `json:"end_line"`
		}
		if err := json.Unmarshal(args, &params); err != nil {
			s.sendError(id, -32602, "Missing dossier_id or artifact_id", nil)
			return
		}
		res, err = s.svc.ReadArtifact(ctx, core.ReadArtifactReq{
			DossierID:  params.DossierID,
			ArtifactID: params.ArtifactID,
			Fragment:   params.Fragment,
			StartLine:  params.StartLine,
			EndLine:    params.EndLine,
		})

	case "dossier_artifacts":
		var params struct {
			DossierID string `json:"dossier_id"`
		}
		if err := json.Unmarshal(args, &params); err != nil {
			s.sendError(id, -32602, "Missing dossier_id", nil)
			return
		}
		res, err = s.svc.ListArtifacts(ctx, core.ListArtifactsReq{DossierID: params.DossierID})

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
		if err == nil {
			s.triggerSync()
		}

	case "dossier_promote":
		var params struct {
			Name                   string   `json:"name"`
			Description            string   `json:"description"`
			Priority               string   `json:"priority"`
			DistilledStateMarkdown string   `json:"distilled_state_markdown"`
			FromFilePath           string   `json:"from_file_path"`
			SessionContent         string   `json:"session_content"`
			Lead                   string   `json:"lead"`
			Interfaces             []string `json:"interfaces"`
			Force                  bool     `json:"force"`
		}
		if err := json.Unmarshal(args, &params); err != nil {
			s.sendError(id, -32602, "Invalid params", nil)
			return
		}
		res, err = s.svc.Promote(ctx, core.PromoteReq{
			Name:                   params.Name,
			Description:            params.Description,
			Priority:               core.Priority(params.Priority),
			DistilledStateMarkdown: params.DistilledStateMarkdown,
			FromFilePath:           params.FromFilePath,
			Content:                params.SessionContent,
			Lead:                   params.Lead,
			Interfaces:             params.Interfaces,
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
			ID           string `json:"id"`
			SessionID    string `json:"session_id"`
			IncludeGuide bool   `json:"include_guide"`
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
					GuideRef              string      `json:"distillation_guide_ref,omitempty"`
					OperatingInstructions string      `json:"operating_instructions,omitempty"`
				}
				// The Guide is sent once per session (see Service.GuideForSession).
				// When it is suppressed, say where it went: an absent field would
				// otherwise read as "this store has no Guide", and the agent needs
				// to know it is still bound by one it can re-read on demand.
				// Once-per-session delivery infers "the Guide is still in
				// context" from session-start events, which cannot see a long
				// session evicting it without a compaction. include_guide is the
				// recovery path for exactly that case: the agent that notices it
				// has lost the rules can ask for them back before it writes,
				// instead of the loss being undetectable.
				guide := ""
				if params.IncludeGuide {
					guide = s.svc.GetGuide()
				} else {
					guide = s.svc.GuideForSession(sid)
				}
				resp := SessionResponse{
					State:                 res.Data,
					Guide:                 guide,
					OperatingInstructions: s.svc.GetInstructions(),
				}
				if resp.Guide == "" {
					// An empty Guide has two very different causes, and reporting
					// the wrong one is worse than reporting nothing: suppression
					// means the rules are in force and already in context, while
					// an unreadable store copy means there are no rules loaded at
					// all. Distinguish them rather than assuming suppression.
					if s.svc.GetGuide() == "" {
						resp.GuideRef = "WARNING: the Distillation Guide is unavailable — ~/.dossier/context/guide.md is missing or unreadable, so no distillation rules are loaded for this session. Run `dossier init` to restore it; until then, treat saves as unguided and say so."
					} else {
						resp.GuideRef = "Distillation Guide already delivered in this session; it remains in force. If it has left your context, re-read ~/.dossier/context/guide.md or call dossier_session with include_guide: true — do not write without it."
					}
				}
				res.Data = resp
			}
		}

	case "dossier_update":
		var params struct {
			ID          string   `json:"id"`
			Name        *string  `json:"name"`
			Description *string  `json:"description"`
			Status      *string  `json:"status"`
			Lead        *string  `json:"lead"`
			NextAction  *string  `json:"next_action"`
			Priority    string   `json:"priority"`
			DueDate     string   `json:"due_date"`
			Interfaces  []string `json:"interfaces"`
		}
		if err := json.Unmarshal(args, &params); err != nil {
			s.sendError(id, -32602, "Invalid params", nil)
			return
		}
		updates := map[string]any{}
		if params.Name != nil {
			updates["name"] = *params.Name
		}
		if params.Description != nil {
			updates["description"] = *params.Description
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
		if params.Priority != "" {
			updates["priority"] = params.Priority
		}
		if params.DueDate != "" {
			updates["due_date"] = params.DueDate
		}
		if params.Interfaces != nil {
			updates["interfaces"] = params.Interfaces
		}
		res, err = s.svc.Save(ctx, core.SaveReq{
			ID:                 params.ID,
			FrontmatterUpdates: updates,
		})

	case "dossier_rename":
		var params struct {
			ID           string `json:"id"`
			NewSlug      string `json:"new_slug"`
			BaseRevision string `json:"base_revision"`
		}
		if err := json.Unmarshal(args, &params); err != nil || params.ID == "" || params.NewSlug == "" || params.BaseRevision == "" {
			s.sendError(id, -32602, "id, new_slug, and base_revision are required", nil)
			return
		}
		res, err = s.svc.RenameSlug(ctx, core.RenameSlugReq{
			ID: params.ID, NewSlug: params.NewSlug, BaseRevision: core.Revision(params.BaseRevision),
		})
		if err == nil {
			s.triggerSync()
		}

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
