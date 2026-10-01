package core

import (
	"context"
	"sort"
	"strings"
	"time"
)

type ChangeItem struct {
	DossierID string    `json:"dossier_id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	Revision  string    `json:"revision,omitempty"`
	Actor     string    `json:"actor"`
	Event     string    `json:"event"`
	Summary   string    `json:"summary"`
	TS        time.Time `json:"ts"`
}

// Changes derives a chronological feed from existing audit shards; it creates
// no index and filters strictly after the supplied timestamp.
func (s *Service) Changes(ctx context.Context, since time.Time) ([]ChangeItem, error) {
	listed, err := s.store.List("all")
	if err != nil {
		return nil, err
	}
	var changes []ChangeItem
	for _, item := range listed {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		events, err := s.store.ReadAuditLog(item.ID)
		if err != nil {
			return nil, err
		}
		for _, event := range events {
			if !event.TS.After(since) {
				continue
			}
			actor := event.Actor
			if actor == "" {
				actor = "human:" + event.Author
				if event.Author == "" {
					actor = "unknown"
				}
			}
			summary := strings.TrimSpace(event.Message)
			if summary == "" {
				summary = strings.ReplaceAll(event.Event, "_", " ")
			}
			changes = append(changes, ChangeItem{DossierID: item.ID, Name: item.Name, Slug: item.Slug, Revision: event.AfterRevision, Actor: actor, Event: event.Event, Summary: summary, TS: event.TS})
		}
	}
	sort.SliceStable(changes, func(i, j int) bool { return changes[i].TS.Before(changes[j].TS) })
	return changes, nil
}
