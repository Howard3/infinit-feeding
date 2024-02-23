package main

import (
	"context"

	"github.com/Howard3/gosignal/drivers/queue"
	_ "github.com/mattn/go-sqlite3"

	"geevly/internal/infrastructure"
	"geevly/internal/student"

	studentpb "geevly/events/gen/proto/go"
)

func main() {
	mq := queue.MemoryQueue{}

	// configure a sqlite connection
	studentConn := infrastructure.SQLConnection{
		Type: "sqlite3",
		URI:  "./student.db",
	}

	// setup the repositories
	studentRepo := student.NewRepository(studentConn, &mq)
	studentService := student.NewStudentService(studentRepo)

	ctx := context.Background()

	newStudent, err := studentService.CreateStudent(ctx, &studentpb.Student_Create{
		FirstName: "John",
		LastName:  "Doe",
		DateOfBirth: &studentpb.Date{
			Year:  1990,
			Month: 11,
			Day:   1,
		},
	})

	if err != nil {
		panic(err)
	}

	updatedStudent, err := studentService.UpdateStudent(ctx, &studentpb.Student_Update{
		StudentId: newStudent.GetStudentId(),
		FirstName: "John",
		LastName:  "Smith",
		DateOfBirth: &studentpb.Date{
			Year:  1992,
			Month: 11,
			Day:   1,
		},
		Version: newStudent.GetVersion(),
	})

	if err != nil {
		panic(err)
	}

	_, err = studentService.SetStatus(ctx, &studentpb.Student_SetStatus{
		StudentId: updatedStudent.GetStudentId(),
		Status:    studentpb.Student_ACTIVE,
		Version:   updatedStudent.GetVersion(),
	})
}
