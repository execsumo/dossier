package core

// Priority is the persisted attention level of a dossier. Values are ordered
// from lowest to highest attention: low, medium, high, max.
type Priority string

const (
	PriorityLow    Priority = "low"
	PriorityMedium Priority = "medium"
	PriorityHigh   Priority = "high"
	PriorityMax    Priority = "max"
)

// IsValid reports whether p is one of the canonical persisted priority values.
func (p Priority) IsValid() bool {
	switch p {
	case PriorityLow, PriorityMedium, PriorityHigh, PriorityMax:
		return true
	}
	return false
}
