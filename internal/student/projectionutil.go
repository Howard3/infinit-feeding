package student

import (
	"fmt"
	"sort"
	"strconv"
)

// HasStudentAggID exposes the aggregate student ID carried by a projection row.
// This is the aggregate's ID-as-string, not the student's LRN.
type HasStudentAggID interface {
	GetStudentAggID() string
}

// Interface implementations on projection structs

// ProjectedStudentHealth implements HasStudentAggID
func (p ProjectedStudentHealth) GetStudentAggID() string { return p.StudentID }

// ProjectedStudentGrade implements HasStudentAggID
func (p ProjectedStudentGrade) GetStudentAggID() string { return p.StudentID }

// CollectDistinctStudentAggIDs returns a sorted unique list of student aggregate IDs from any projection slice.
func CollectDistinctStudentAggIDs[T HasStudentAggID](recs []T) []string {
	if len(recs) == 0 {
		return nil
	}
	m := make(map[uint64]struct{}, len(recs))
	for _, r := range recs {
		id := r.GetStudentAggID()

		uintID, err := strconv.ParseUint(id, 10, 64)
		if err != nil {
			continue
		}

		m[uintID] = struct{}{}
	}
	out := make([]string, 0, len(m))
	for id := range m {
		out = append(out, fmt.Sprintf("%v", id))
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
