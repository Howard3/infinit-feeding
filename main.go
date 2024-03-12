package main

import (
	"context"
	"io/fs"
	"os"

	"github.com/Howard3/gosignal/drivers/queue"
	_ "github.com/mattn/go-sqlite3"

	"geevly/internal/file"
	"geevly/internal/infrastructure"
	"geevly/internal/school"
	"geevly/internal/student"
	"geevly/internal/user"
	"geevly/internal/webapi"
)

func getStaticFS() fs.FS {
	// TODO: support embedded for prod
	return os.DirFS("./static")
}

func main() {
	ctx := context.Background()

	mq := queue.MemoryQueue{} // TODO: swap to nats
	s3 := infrastructure.S3Storage{}

	// configure a sqlite connection
	studentConn := infrastructure.SQLConnection{
		Type: "sqlite3",
		URI:  "./student.db",
	}

	schoolConn := infrastructure.SQLConnection{
		Type: "sqlite3",
		URI:  "./school.db",
	}

	userConn := infrastructure.SQLConnection{
		Type: "sqlite3",
		URI:  "./user.db",
	}

	fileConn := infrastructure.SQLConnection{
		Type: "sqlite3",
		URI:  "./file.db",
	}

	userRepo := user.NewRepository(userConn, &mq)
	userService := user.NewService(userRepo)

	fileRepo := file.NewRepository(fileConn, &mq)
	fileService := file.NewService(fileRepo, s3)

	schoolRepo := school.NewRepository(schoolConn, &mq)
	schoolService := school.NewService(schoolRepo)

	studentACL := webapi.NewAclStudents(schoolService)

	studentRepo := student.NewRepository(studentConn, &mq)
	studentService := student.NewStudentService(studentRepo, studentACL)

	// TODO: load config from env, put here.
	server := webapi.Server{
		StaticFS:   getStaticFS(),
		StudentSvc: studentService,
		SchoolSvc:  schoolService,
		UserSvc:    userService,
		FileSvc:    fileService,
	}
	server.Start(ctx)
}
