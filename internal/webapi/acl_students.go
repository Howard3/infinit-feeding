package webapi

import (
	"context"
	"errors"
	"fmt"
	"geevly/internal/file"
	"geevly/internal/school"
	"strconv"
)

// ErrSchoolIDInvalid is an error that is returned when a school ID is invalid
var ErrSchoolIDInvalid = fmt.Errorf("error validating school")

type AclStudents struct {
	schoolSvc *school.Service
	fileSvc   *file.Service
}

// ValidateSchoolID validates a school ID, returns an error if the school ID is invalid
func (as AclStudents) ValidateSchoolID(ctx context.Context, schoolID string) error {
	id, err := strconv.ParseUint(schoolID, 10, 64)
	if err != nil {
		return errors.Join(ErrSchoolIDInvalid, err)
	}

	return as.schoolSvc.ValidateSchoolID(ctx, id)
}

// ValidatePhotoID validates a photo from the file domain
func (as AclStudents) ValidatePhotoID(ctx context.Context, photoID string) error {
	return as.fileSvc.ValidateFileID(ctx, photoID)
} // don't run in goroutine, it's likely needed immediately after via "ValidateFileID"

// NewAclStudents creates a new AclStudents instance
func NewAclStudents(schoolSvc *school.Service, fileSvc *file.Service) AclStudents {
	return AclStudents{
		schoolSvc: schoolSvc,
		fileSvc:   fileSvc,
	}
}
