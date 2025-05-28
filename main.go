package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"

	"github.com/Howard3/gosignal/drivers/queue"
	"github.com/clerkinc/clerk-sdk-go/clerk"

	"geevly/internal/bulk_upload"
	"geevly/internal/file"
	"geevly/internal/infrastructure"
	"geevly/internal/school"
	"geevly/internal/student"
	"geevly/internal/webapi"

	_ "github.com/mattn/go-sqlite3"
	_ "github.com/tursodatabase/libsql-client-go/libsql"

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

	clerkClient, err := clerk.NewClient(os.Getenv("CLERK_SECRET_KEY"))
	if err != nil {
		panic(fmt.Errorf("error creating clerk client: %w", err))
	}

	// configure a sqlite connection
	db := infrastructure.SQLConnection{
		Type: "libsql",
		URI:  os.Getenv("DB_URI"),
	}

	fileRepo := file.NewRepository(db, &mq)
	fileService := file.NewService(fileRepo, &s3)

	schoolRepo := school.NewRepository(db, &mq)
	schoolService := school.NewService(schoolRepo)

	studentACL := webapi.NewAclStudents(schoolService, fileService)

	studentRepo := student.NewRepository(db, &mq)
	studentService := student.NewStudentService(studentRepo, studentACL)

	bulkUploadACL := webapi.NewBulkUploadACL(fileService)
	bulkUploadRepo := bulk_upload.NewRepository(db, &mq)
	bulkUploadService := bulk_upload.NewService(bulkUploadRepo, bulkUploadACL)

	server := webapi.NewServer(":3000", getStaticFS(), studentService, schoolService, fileService, bulkUploadService, clerkClient)
	server.Start(ctx)
}
