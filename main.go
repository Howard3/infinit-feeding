package main

import (
	"context"
	"io/fs"
	"os"

	"github.com/Howard3/gosignal/drivers/queue"
	_ "github.com/mattn/go-sqlite3"

	"geevly/internal/infrastructure"
	"geevly/internal/student"
	"geevly/internal/webapi"
)

func getStaticFS() fs.FS {
	// TODO: support embedded for prod
	return os.DirFS("./static")
}

func main() {
	ctx := context.Background()

	mq := queue.MemoryQueue{}

	// configure a sqlite connection
	studentConn := infrastructure.SQLConnection{
		Type: "sqlite3",
		URI:  "./student.db",
	}

	// setup the repositories
	studentRepo := student.NewRepository(studentConn, &mq)
	studentService := student.NewStudentService(studentRepo)

	// TODO: load config from env, put here.
	server := webapi.Server{
		StaticFS:    getStaticFS(),
		StudentSvc:  studentService,
	}
	server.Start(ctx)
}
