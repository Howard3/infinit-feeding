package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"geevly/internal/infrastructure"
	"geevly/internal/student"

	"github.com/Howard3/gosignal/drivers/queue"
	"github.com/joho/godotenv"
	_ "github.com/mattn/go-sqlite3"
	_ "github.com/tursodatabase/libsql-client-go/libsql"
)

func main() {
	_ = godotenv.Load()
	ctx := context.Background()

	if len(os.Args) < 2 {
		fmt.Println("Usage: debug <command> [args]")
		fmt.Println("Commands:")
		fmt.Println("  health <student_id>     - Show health assessment status for a student")
		fmt.Println("  search <name>           - Search for students by name")
		os.Exit(1)
	}

	mq := queue.MemoryQueue{}
	db := infrastructure.SQLConnection{
		Type: "libsql",
		URI:  os.Getenv("DB_URI"),
	}

	studentRepo := student.NewRepository(db, &mq)

	cmd := os.Args[1]
	switch cmd {
	case "health":
		if len(os.Args) < 3 {
			fmt.Println("Usage: debug health <student_id>")
			os.Exit(1)
		}
		studentID, err := strconv.ParseUint(os.Args[2], 10, 64)
		if err != nil {
			fmt.Printf("Invalid student ID: %v\n", err)
			os.Exit(1)
		}
		showHealthStatus(ctx, studentRepo, studentID)

	case "search":
		if len(os.Args) < 3 {
			fmt.Println("Usage: debug search <name>")
			os.Exit(1)
		}
		name := strings.Join(os.Args[2:], " ")
		searchStudents(ctx, studentRepo, name)

	default:
		fmt.Printf("Unknown command: %s\n", cmd)
		os.Exit(1)
	}
}

func showHealthStatus(ctx context.Context, repo student.Repository, studentID uint64) {
	svc := student.NewStudentService(repo, &noopACL{})

	agg, err := svc.GetStudent(ctx, studentID)
	if err != nil {
		fmt.Printf("Error loading student: %v\n", err)
		os.Exit(1)
	}

	s := agg.GetStudent()
	fmt.Printf("\n=== Student Information ===\n")
	fmt.Printf("ID:          %d\n", studentID)
	fmt.Printf("Name:        %s %s\n", s.FirstName, s.LastName)
	fmt.Printf("DOB:         %d-%02d-%02d\n", s.DateOfBirth.Year, s.DateOfBirth.Month, s.DateOfBirth.Day)
	fmt.Printf("Sex:         %s\n", s.Sex.String())
	fmt.Printf("School ID:   %s\n", s.SchoolId)

	assessments := agg.GetHealthAssessments()
	if len(assessments) == 0 {
		fmt.Println("\nNo health assessments found.")
		return
	}

	fmt.Printf("\n=== Health Assessments (%d total) ===\n", len(assessments))
	fmt.Println("------------------------------------------------------------------------------------------")
	fmt.Printf("%-12s | %-6s | %-6s | %-6s | %-10s | %-6s | %-20s\n",
		"Date", "Height", "Weight", "BMI", "Age(mo)", "Age(y)", "Status")
	fmt.Println("------------------------------------------------------------------------------------------")

	for _, h := range assessments {
		status := h.NutritionalStatus()
		fmt.Printf("%-12s | %6.1f | %6.1f | %6.2f | %10d | %6.2f | %-20s\n",
			h.AssessmentDate.Format("2006-01-02"),
			h.HeightCm,
			h.WeightKg,
			h.BMI(),
			h.AgeMonths(),
			h.AgeYears(),
			status.String(),
		)
	}
	fmt.Println("------------------------------------------------------------------------------------------")
}

func searchStudents(ctx context.Context, repo student.Repository, name string) {
	svc := student.NewStudentService(repo, &noopACL{})

	result, err := svc.ListStudents(ctx, 50, 1, student.WithNameSearch(name))
	if err != nil {
		fmt.Printf("Error searching students: %v\n", err)
		os.Exit(1)
	}

	if len(result.Students) == 0 {
		fmt.Printf("No students found matching '%s'\n", name)
		return
	}

	fmt.Printf("\n=== Search Results for '%s' (%d found) ===\n", name, len(result.Students))
	fmt.Println("--------------------------------------------------------------------------------")
	fmt.Printf("%-8s | %-30s | %-12s | %-10s\n", "ID", "Name", "DOB", "School ID")
	fmt.Println("--------------------------------------------------------------------------------")

	for _, s := range result.Students {
		fmt.Printf("%-8d | %-30s | %-12s | %-10s\n",
			s.ID,
			fmt.Sprintf("%s %s", s.FirstName, s.LastName),
			s.DateOfBirth.Format("2006-01-02"),
			s.SchoolID,
		)
	}
	fmt.Println("--------------------------------------------------------------------------------")
}

type noopACL struct{}

func (n *noopACL) ValidateSchoolID(ctx context.Context, schoolID string) error { return nil }
func (n *noopACL) ValidatePhotoID(ctx context.Context, photoID string) error   { return nil }
