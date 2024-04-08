package main

import (
	"context"
	"io/fs"
	"os"

	"github.com/Howard3/gosignal/drivers/queue"
	_ "github.com/tursodatabase/libsql-client-go/libsql"

	"geevly/internal/file"
	"geevly/internal/infrastructure"
	"geevly/internal/school"
	"geevly/internal/student"
	"geevly/internal/user"
	"geevly/internal/webapi"

	"github.com/joho/godotenv"
)

func getStaticFS() fs.FS {
	// TODO: support embedded for prod
	return os.DirFS("./static")
}

func main() {
	_ = godotenv.Load()
	ctx := context.Background()

	mq := queue.MemoryQueue{} // TODO: swap to nats
	s3 := infrastructure.S3Storage{
		Endpoint:     os.Getenv("S3_ENDPOINT"),
		AccessKey:    os.Getenv("S3_ACCESS_KEY"),
		SecretKey:    os.Getenv("S3_SECRET_KEY"),
		S3BucketName: os.Getenv("S3_BUCKET_NAME"),
		Region:       os.Getenv("S3_REGION"),
	}

	// configure a sqlite connection
	db := infrastructure.SQLConnection{
		Type: "libsql",
		URI:  os.Getenv("DB_URI"),
	}

	userRepo := user.NewRepository(db, &mq)
	userService := user.NewService(userRepo)

	fileRepo := file.NewRepository(db, &mq)
	fileService := file.NewService(fileRepo, &s3)

	schoolRepo := school.NewRepository(db, &mq)
	schoolService := school.NewService(schoolRepo)

	studentACL := webapi.NewAclStudents(schoolService, fileService)

	studentRepo := student.NewRepository(db, &mq)
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
