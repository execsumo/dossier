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

// PriorityFromLegacyMatrix converts the former importance/urgency matrix to
// canonical priority. The old schema normalized a missing or unknown dimension
// toward attention (to "high"), so compatibility reads preserve that behavior.
func PriorityFromLegacyMatrix(importance, urgency string) Priority {
	if importance != "low" {
		importance = "high"
	}
	if urgency != "low" {
		urgency = "high"
	}

	switch {
	case importance == "high" && urgency == "high":
		return PriorityMax
	case importance == "high" && urgency == "low":
		return PriorityHigh
	case importance == "low" && urgency == "high":
		return PriorityMedium
	default:
		return PriorityLow
	}
}
