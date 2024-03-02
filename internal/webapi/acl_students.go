package webapi

import (
	"context"
	"errors"
	"fmt"
	"geevly/internal/school"
	"strconv"
)

// ErrSchoolIDInvalid is an error that is returned when a school ID is invalid
var ErrSchoolIDInvalid = fmt.Errorf("error validating school")

type AclStudents struct {
	schoolSvc *school.Service
}

// ValidateSchoolID validates a school ID, returns an error if the school ID is invalid
func (as AclStudents) ValidateSchoolID(ctx context.Context, schoolID string) error {
	id, err := strconv.ParseUint(schoolID, 10, 64)
	if err != nil {
		return errors.Join(ErrSchoolIDInvalid, err)
	}

	return as.schoolSvc.ValidateSchoolID(ctx, id)
}

// NewAclStudents creates a new AclStudents instance
func NewAclStudents(schoolSvc *school.Service) AclStudents {
	return AclStudents{
		schoolSvc: schoolSvc,
	}
}
