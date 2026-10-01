package core

import (
	"fmt"
	"strings"
)

type ActorKind string

const (
	ActorHuman  ActorKind = "human"
	ActorAgent  ActorKind = "agent"
	ActorSystem ActorKind = "system"
)

// NormalizeActor supplies the historical human identity when callers omit an
// explicit actor. Actor attribution is provenance, not authentication.
func NormalizeActor(actor, author string) string {
	actor = strings.TrimSpace(actor)
	if actor == "" {
		identity := NormalizeUsername(author)
		if identity == "" {
			identity = "unknown"
		}
		return "human:" + identity
	}
	return actor
}

func ActorIsHuman(actor string) bool { return strings.HasPrefix(strings.TrimSpace(actor), "human:") }
func ActorIsAgent(actor string) bool { return strings.HasPrefix(strings.TrimSpace(actor), "agent:") }

func ValidateActor(actor string) error {
	kind, identity, ok := strings.Cut(strings.TrimSpace(actor), ":")
	if !ok || strings.TrimSpace(identity) == "" {
		return fmt.Errorf("actor must be human:<identity>, agent:<slug>, or system:<name>")
	}
	switch ActorKind(kind) {
	case ActorHuman, ActorSystem:
		if strings.ContainsAny(identity, "\r\n") {
			return fmt.Errorf("actor identity must be a single line")
		}
		return nil
	case ActorAgent:
		if err := ValidateCanonicalSlug(identity); err != nil {
			return fmt.Errorf("agent actor must use a stable slug: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("unsupported actor kind %q", kind)
	}
}

// Authorize enforces the core human-only action policy; it is a guard against
// accidental drift, not an authentication boundary.
func Authorize(actor, action string) error {
	if err := ValidateActor(actor); err != nil {
		return err
	}
	switch action {
	case "set_done", "accept_delegation", "accept_agent_proposal", "change_standing_orders", "rename", "merge":
		if !ActorIsHuman(actor) {
			return fmt.Errorf("%s requires a human actor", action)
		}
	}
	return nil
}
