package infrastructure

import (
	"bytes"
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

type S3Storage struct {
	S3BucketName string
	Region       string
	AccessKey    string
	SecretKey    string
	Endpoint     string // For MinIO or other S3-compatible services
}

func (s *S3Storage) newSession() (*session.Session, error) {
	sess, err := session.NewSession(&aws.Config{
		Region:           aws.String(s.Region),
		Credentials:      credentials.NewStaticCredentials(s.AccessKey, s.SecretKey, ""),
		Endpoint:         aws.String(s.Endpoint),
		S3ForcePathStyle: aws.Bool(true), // Necessary for MinIO and other S3-compatible services
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS session: %v", err)
	}
	return sess, nil
}

func (s *S3Storage) StoreFile(ctx context.Context, domainReference, id string, fileData []byte) error {
	sess, err := s.newSession()
	if err != nil {
		return err
	}

	s3Client := s3.New(sess)

	_, err = s3Client.PutObjectWithContext(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.S3BucketName),
		Key:    aws.String(fmt.Sprintf("%s/%s", domainReference, id)),
		Body:   bytes.NewReader(fileData),
	})
	if err != nil {
		return fmt.Errorf("failed to put object: %v", err)
	}

	return nil
}

func (s *S3Storage) RetrieveFile(ctx context.Context, domainReference, fileId string) ([]byte, error) {
	sess, err := s.newSession()
	if err != nil {
		return nil, err
	}

	s3Client := s3.New(sess)

	getObjectOutput, err := s3Client.GetObjectWithContext(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.S3BucketName),
		Key:    aws.String(fmt.Sprintf("%s/%s", domainReference, fileId)),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get object: %v", err)
	}
	defer getObjectOutput.Body.Close()

	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(getObjectOutput.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read object data: %v", err)
	}

	return buf.Bytes(), nil
}

func (s *S3Storage) DeleteFile(ctx context.Context, domainReference, fileId string) error {
	sess, err := s.newSession()
	if err != nil {
		return err
	}

	s3Client := s3.New(sess)

	_, err = s3Client.DeleteObjectWithContext(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.S3BucketName),
		Key:    aws.String(fmt.Sprintf("%s/%s", domainReference, fileId)),
	})
	if err != nil {
		return fmt.Errorf("failed to delete object: %v", err)
	}

	return nil
}
