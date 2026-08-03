package bulk_domains

import (
	"io"
	"testing"
)

func TestNewCSVReader_WithBOM(t *testing.T) {
	data := append([]byte{0xEF, 0xBB, 0xBF}, []byte("First Name,Last Name\nAda,Lovelace\n")...)
	reader := newCSVReader(data)

	header, err := reader.Read()
	if err != nil {
		t.Fatalf("Read header: %v", err)
	}
	if header[0] != "First Name" {
		t.Fatalf("first header = %q, want %q", header[0], "First Name")
	}
}

func TestNewCSVReader_WithoutBOM(t *testing.T) {
	data := []byte("First Name,Last Name\nAda,Lovelace\n")
	reader := newCSVReader(data)

	header, err := reader.Read()
	if err != nil {
		t.Fatalf("Read header: %v", err)
	}
	if header[0] != "First Name" {
		t.Fatalf("first header = %q, want %q", header[0], "First Name")
	}
}

func TestNewCSVReader_Empty(t *testing.T) {
	reader := newCSVReader(nil)
	_, err := reader.Read()
	if err != io.EOF {
		t.Fatalf("Read empty = %v, want io.EOF", err)
	}
}
