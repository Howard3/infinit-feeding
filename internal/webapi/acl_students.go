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

// AclStudents is a service that validates student data. It's an anti-corruption layer
type AclStudents struct {
	schoolService *school.Service
	fileService   *file.Service
}

// ValidateSchoolID validates a school ID, returns an error if the school ID is invalid
func (as AclStudents) ValidateSchoolID(ctx context.Context, schoolID string) error {
	id, err := strconv.ParseUint(schoolID, 10, 64)
	if err != nil {
		return errors.Join(ErrSchoolIDInvalid, err)
	}

	return as.schoolService.ValidateSchoolID(ctx, id)
}

// ValidatePhotoID validates a photo from the file domain
func (as AclStudents) ValidatePhotoID(ctx context.Context, photoID string) error {
	return as.fileService.ValidateFileID(ctx, photoID)
}

// NewAclStudents creates a new AclStudents instance
func NewAclStudents(schoolService *school.Service, fileService *file.Service) AclStudents {
	return AclStudents{
		schoolService: schoolService,
		fileService:   fileService,
	}
}
