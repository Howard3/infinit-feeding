package bulk_domains

import (
	"archive/zip"
	"bytes"
	"context"
	"strings"
	"testing"

	"geevly/gen/go/eda"
	"geevly/internal/bulk_upload"
)

const newStudentsTemplateHeader = "First Name,Last Name,LRN,Grade Level,Date of Birth,Gender,Status,Sponsorship"

func TestValidateUpload_LRNNumberHeaderIsValidationError(t *testing.T) {
	csv := "" +
		"First Name,Last Name,LRN NUMBER,Grade Level,Date of Birth,Gender,Status,Sponsorship\n" +
		"Jane,Doe,12345678,3,20120315,F,Active,Not Eligible\n"
	result := validateNewStudentsCSV(t, csv, nil)

	if result.IsValid {
		t.Fatal("IsValid = true, want false")
	}
	if len(result.Errors) != 1 {
		t.Fatalf("len(Errors) = %d, want 1: %+v", len(result.Errors), result.Errors)
	}
	err := result.Errors[0]
	if err.Context != eda.BulkUpload_ValidationError_METADATA_FIELD {
		t.Fatalf("Context = %s, want METADATA_FIELD", err.Context)
	}
	if err.Field != "headers" {
		t.Fatalf("Field = %q, want headers", err.Field)
	}
	want := "Missing required columns: LRN. Found columns: First Name, Last Name, LRN NUMBER, Grade Level, Date of Birth, Gender, Status, Sponsorship"
	if err.Message != want {
		t.Fatalf("Message = %q, want %q", err.Message, want)
	}
	for _, e := range result.Errors {
		if e.Field == "rows" || strings.Contains(e.Message, "photo") || strings.Contains(e.Message, "duplicate LRN") {
			t.Fatalf("row walking ran: %+v", result.Errors)
		}
	}
}

func TestValidateUpload_TemplateHeaderWithPhoto(t *testing.T) {
	csv := newStudentsTemplateHeader + "\n" +
		"Jane,Doe,12345678,3,20120315,F,Active,Not Eligible\n"
	result := validateNewStudentsCSV(t, csv, map[string][]byte{
		"photos/12345678.jpg": []byte("jpeg-bytes"),
	})

	if !result.IsValid {
		t.Fatalf("IsValid = false, errors: %+v", result.Errors)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("Errors = %+v, want none", result.Errors)
	}
}

func TestValidateUpload_ShortDataRow(t *testing.T) {
	csv := newStudentsTemplateHeader + "\nJane\n"
	result := validateNewStudentsCSV(t, csv, nil)

	if result.IsValid {
		t.Fatal("IsValid = true, want false")
	}
	var rowErr *eda.BulkUpload_ValidationError
	for _, err := range result.Errors {
		if err.Field == "rows" && strings.Contains(err.Message, "row 2") {
			rowErr = err
			break
		}
	}
	if rowErr == nil {
		t.Fatalf("no rows error mentioning row 2: %+v", result.Errors)
	}
	if rowErr.Context != eda.BulkUpload_ValidationError_CSV_DATA {
		t.Fatalf("Context = %s, want CSV_DATA", rowErr.Context)
	}
	want := "row 2 has 1 columns, need at least 8"
	if rowErr.Message != want {
		t.Fatalf("Message = %q, want %q", rowErr.Message, want)
	}
}

func TestValidateUpload_MissingLRNAndGender(t *testing.T) {
	csv := "" +
		"First Name,Last Name,LRN NUMBER,Grade Level,Date of Birth,Sex,Status,Sponsorship\n" +
		"Jane,Doe,12345678,3,20120315,F,Active,Not Eligible\n"
	result := validateNewStudentsCSV(t, csv, nil)

	if result.IsValid {
		t.Fatal("IsValid = true, want false")
	}
	if len(result.Errors) != 1 {
		t.Fatalf("len(Errors) = %d, want 1: %+v", len(result.Errors), result.Errors)
	}
	err := result.Errors[0]
	if err.Field != "headers" {
		t.Fatalf("Field = %q, want headers", err.Field)
	}
	want := "Missing required columns: LRN, Gender. Found columns: First Name, Last Name, LRN NUMBER, Grade Level, Date of Birth, Sex, Status, Sponsorship"
	if err.Message != want {
		t.Fatalf("Message = %q, want %q", err.Message, want)
	}
}

func validateNewStudentsCSV(t *testing.T, csv string, photos map[string][]byte) *ValidationResult {
	t.Helper()

	agg := &bulk_upload.Aggregate{}
	if _, err := agg.Create(&eda.BulkUpload_Create{
		TargetDomain:   eda.BulkUpload_NEW_STUDENTS,
		FileId:         "file",
		UploadMetadata: map[string]string{"school_id": "1"},
	}); err != nil {
		t.Fatalf("create aggregate: %v", err)
	}

	domain := NewNewStudentsDomain(nil)
	return domain.ValidateUpload(context.Background(), agg, zipWithStudentsCSV(t, csv, photos))
}

func zipWithStudentsCSV(t *testing.T, csv string, photos map[string][]byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("students.csv")
	if err != nil {
		t.Fatalf("create students.csv: %v", err)
	}
	if _, err := w.Write([]byte(csv)); err != nil {
		t.Fatalf("write students.csv: %v", err)
	}
	for name, data := range photos {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}
