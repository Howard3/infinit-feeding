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

	studentService.CreateStudent(ctx, &studentpb.AddStudentEvent{
		FirstName: "John",
		LastName:  "Doe",
		DateOfBirth: &studentpb.Date{
			Year:  1990,
			Month: 11,
			Day:   1,
		},
	})

}
