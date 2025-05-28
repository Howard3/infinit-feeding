package webapi

import (
	"context"
	"geevly/internal/file"
)

// BulkUploadACL is an anti-corruption layer that adapts the web API to the bulk upload domain
type BulkUploadACL struct {
	fileService *file.Service
}

// NewBulkUploadACL creates a new anti-corruption layer for bulk uploads
func NewBulkUploadACL(fileService *file.Service) *BulkUploadACL {
	return &BulkUploadACL{
		fileService: fileService,
	}
}

// ValidateFileID implements the AntiCorruptionLayer interface for the bulk upload service
// by delegating to the file service's ValidateFileID method
func (a *BulkUploadACL) ValidateFileID(ctx context.Context, fileID string) error {
	return a.fileService.ValidateFileID(ctx, fileID)
}
