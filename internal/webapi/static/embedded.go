package static

import (
	"archive/zip"
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
)

//go:embed bulk_templates
var BulkTemplatesRoot embed.FS

// BulkTemplateType represents the type of template (CSV, ZIP, etc.)
type BulkTemplateType string

const (
	CSV BulkTemplateType = "csv"
	ZIP BulkTemplateType = "zip"
)

// BulkTemplateInfo contains information about the template
type BulkTemplateInfo struct {
	Filename    string // Suggested filename for the download
	ContentType string // MIME type
	Data        []byte // The actual template data
}

// GetBulkTemplateInfo returns template information for a specific domain
func GetBulkTemplateInfo(domain string, templateType BulkTemplateType) (BulkTemplateInfo, error) {
	switch domain {
	case "new_students":
		return getNewStudentsTemplate(templateType)
	case "health_assessment":
		return getHealthAssessmentTemplate(templateType)
	case "grades":
		return getGradesTemplate(templateType)
	case "attendance":
		return getAttendanceTemplate(templateType)
	default:
		return BulkTemplateInfo{}, fmt.Errorf("unknown domain: %s", domain)
	}
}

// getNewStudentsTemplate returns
func getNewStudentsTemplate(templateType BulkTemplateType) (BulkTemplateInfo, error) {
	// Generate a ZIP with template CSV, README and photos directory
	zipData, err := createNewStudentsZipTemplate()
	if err != nil {
		return BulkTemplateInfo{}, err
	}
	return BulkTemplateInfo{
		Filename:    "new_students_template.zip",
		ContentType: "application/zip",
		Data:        zipData,
	}, nil
}

// createNewStudentsZipTemplate generates a ZIP file with new students template contents
func createNewStudentsZipTemplate() ([]byte, error) {
	BulkTemplates, err := bulkTemplates()
	if err != nil {
		return nil, err
	}

	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)

	// DEBUG: LIST BulkTemplates directory
	slog.Info("Listing BulkTemplates directory", "dir", BulkTemplates)
	fs.WalkDir(BulkTemplates, ".", func(path string, entry fs.DirEntry, err error) error {
		slog.Info("found", "path", path, "entry", entry)
		return nil
	})

	// Add the CSV template
	csvData, err := fs.ReadFile(BulkTemplates, "new_students/new_students.csv")
	if err != nil {
		return nil, err
	}
	csvWriter, err := w.Create("students.csv")
	if err != nil {
		return nil, err
	}
	if _, err = csvWriter.Write(csvData); err != nil {
		return nil, err
	}

	// Add the README
	readmeData, err := fs.ReadFile(BulkTemplates, "new_students/README.txt")
	if err != nil {
		return nil, err
	}
	readmeWriter, err := w.Create("README.txt")
	if err != nil {
		return nil, err
	}
	if _, err = readmeWriter.Write(readmeData); err != nil {
		return nil, err
	}

	// Create photos directory with README
	_, err = w.Create("photos/")
	if err != nil {
		return nil, err
	}

	photosReadmeWriter, err := w.Create("photos/README.txt")
	if err != nil {
		return nil, err
	}
	if _, err = photosReadmeWriter.Write([]byte("Place student photos in this directory.\nName each photo using the student's LRN (e.g., 12345678.jpg).")); err != nil {
		return nil, err
	}

	if err := w.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// getHealthAssessmentTemplate returns templates for the health_assessment domain
func getHealthAssessmentTemplate(templateType BulkTemplateType) (BulkTemplateInfo, error) {
	switch templateType {
	case CSV:
		data := []byte(`lrn,height_cm,weight_kg,assessment_date
"ST001",120.5,23.4,"2023-10-15"
"ST002",115.0,21.0,"2023-10-15"`)

		return BulkTemplateInfo{
			Filename:    "health_assessment_template.csv",
			ContentType: "text/csv",
			Data:        data,
		}, nil
	default:
		return BulkTemplateInfo{}, fmt.Errorf("unsupported template type: %s for health_assessment", templateType)
	}
}

// getGradesTemplate returns templates for the grades domain
func getGradesTemplate(templateType BulkTemplateType) (BulkTemplateInfo, error) {
	switch templateType {
	case CSV:
		data := []byte(`LRN,Grade
"12345678",92
"23456789",88
"34567890",90`)

		return BulkTemplateInfo{
			Filename:    "grades_template.csv",
			ContentType: "text/csv",
			Data:        data,
		}, nil
	default:
		return BulkTemplateInfo{}, fmt.Errorf("unsupported template type: %s for grades", templateType)
	}
}

// getAttendanceTemplate returns templates for the attendance domain
func getAttendanceTemplate(templateType BulkTemplateType) (BulkTemplateInfo, error) {
	switch templateType {
	case CSV:
		data := []byte(`student_id,date,status,reason
"ST001","2023-10-01","PRESENT",""
"ST001","2023-10-02","ABSENT","Sick"
"ST001","2023-10-03","PRESENT",""`)

		return BulkTemplateInfo{
			Filename:    "attendance_template.csv",
			ContentType: "text/csv",
			Data:        data,
		}, nil
	default:
		return BulkTemplateInfo{}, fmt.Errorf("unsupported template type: %s for attendance", templateType)
	}
}

func bulkTemplates() (fs.FS, error) {
	bulkTemplates, err := fs.Sub(BulkTemplatesRoot, "bulk_templates")
	if err != nil {
		return nil, fmt.Errorf("sub bulk templates %w", err)
	}
	return bulkTemplates, nil
}
