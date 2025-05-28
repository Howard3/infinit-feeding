package bulk_domains

import (
	"context"
	"geevly/gen/go/eda"
	"geevly/internal/bulk_upload"
	"geevly/internal/file"
	"geevly/internal/school"
	"geevly/internal/student"
	"net/http"
)

// ValidationResult contains the result of a bulk upload validation
type ValidationResult struct {
	IsValid bool
	Errors  []*eda.BulkUpload_ValidationError
}

// BulkUploadDomain defines the interface for domain-specific upload handlers
type BulkUploadDomain interface {
	// ValidateFormData validates domain-specific form data
	ValidateFormData(r *http.Request) (map[string]string, error)

	// UploadFile handles the file upload process
	UploadFile(r *http.Request, fileSvc *file.Service) (string, error)

	// GetTargetDomain returns the EDA domain type
	GetDomain() eda.BulkUpload_Domain

	// GetFileName returns the name of the file field in the form
	GetFileName() string

	// GetMaxFileSize returns the maximum file size in bytes
	GetMaxFileSize() int64

	// ValidateUpload validates the uploaded file against business rules
	ValidateUpload(ctx context.Context, aggregate *bulk_upload.Aggregate, fileBytes []byte) *ValidationResult

	// ProcessUpload handles the upload process for the file
	ProcessUpload(ctx context.Context, aggregate *bulk_upload.Aggregate, svc *bulk_upload.Service, fileBytes []byte) error

	// UndoUpload handles the undo process for the uploaded file
	UndoUpload(ctx context.Context, aggregate *bulk_upload.Aggregate, svc *bulk_upload.Service) error
}

// ServiceRegistry contains all services that domain handlers might need. Redefined here to prevent circular dependencies.
type ServiceRegistry struct {
	SchoolService  *school.Service
	StudentService *student.StudentService
	FileService    *file.Service
}

// DomainRegistry manages domain handlers
type DomainRegistry struct {
	domains map[eda.BulkUpload_Domain]BulkUploadDomain
}

// NewDomainRegistry creates and initializes a new domain registry
func NewDomainRegistry(services *ServiceRegistry) *DomainRegistry {
	registry := &DomainRegistry{
		domains: make(map[eda.BulkUpload_Domain]BulkUploadDomain),
	}

	// Register domains with access to services
	registry.domains[eda.BulkUpload_NEW_STUDENTS] = NewNewStudentsDomain(services)
	registry.domains[eda.BulkUpload_GRADES] = NewGradesDomain(services)
	registry.domains[eda.BulkUpload_HEALTH_ASSESSMENT] = NewHealthAsssementDomain(services)

	return registry
}

// GetDomain retrieves a domain handler by name
func (r *DomainRegistry) GetDomain(domainName eda.BulkUpload_Domain) (BulkUploadDomain, bool) {
	domain, exists := r.domains[domainName]
	return domain, exists
}

// RegisterDomain adds a new domain handler to the registry
func (r *DomainRegistry) RegisterDomain(name eda.BulkUpload_Domain, domain BulkUploadDomain) {
	r.domains[name] = domain
}

// validateHeaders checks if all required columns are present in the header
func validateCSVHeaders(header []string, requiredColumns []string) []string {
	var missingColumns []string
	headerMap := make(map[string]bool)

	for _, col := range header {
		headerMap[col] = true
	}

	for _, required := range requiredColumns {
		if _, ok := headerMap[required]; !ok {
			missingColumns = append(missingColumns, required)
		}
	}

	return missingColumns
}
