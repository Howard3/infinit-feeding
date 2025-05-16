package bulk_domains

import (
	"geevly/gen/go/eda"
	"net/http"
)

// BulkUploadDomain defines the interface for domain-specific upload handlers
type BulkUploadDomain interface {
	// ValidateFormData validates domain-specific form data
	ValidateFormData(r *http.Request) (map[string]string, error)

	// UploadFile handles the file upload process
	UploadFile(r *http.Request, fileSvc FileService) (string, error)

	// GetTargetDomain returns the EDA domain type
	GetDomain() eda.BulkUpload_Domain

	// GetFileName returns the name of the file field in the form
	GetFileName() string

	// GetMaxFileSize returns the maximum file size in bytes
	GetMaxFileSize() int64
}

// FileService defines the interface for file storage operations
type FileService interface {
	CreateFile(ctx http.Request, fileBytes []byte, fileCreate *eda.File_Create) (string, error)
	GetFileBytes(ctx http.Request, domainRef string, fileID string) ([]byte, error)
}

// DomainRegistry manages domain handlers
type DomainRegistry struct {
	domains map[string]BulkUploadDomain
}

// NewDomainRegistry creates and initializes a new domain registry
func NewDomainRegistry() *DomainRegistry {
	registry := &DomainRegistry{
		domains: make(map[string]BulkUploadDomain),
	}

	// Register domains
	registry.domains["new_students"] = &NewStudentsDomain{}
	registry.domains["grades"] = &GradesDomain{}

	return registry
}

// GetDomain retrieves a domain handler by name
func (r *DomainRegistry) GetDomain(domainName string) (BulkUploadDomain, bool) {
	domain, exists := r.domains[domainName]
	return domain, exists
}

// RegisterDomain adds a new domain handler to the registry
func (r *DomainRegistry) RegisterDomain(name string, domain BulkUploadDomain) {
	r.domains[name] = domain
}
