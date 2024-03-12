package infrastructure

import "context"

type S3Storage struct{}

func (s3 S3Storage) StoreFile(ctx context.Context, domainReference string, id string, fileData []byte) error {
	panic("not implemented") // TODO: Implement
}
func (s3 S3Storage) RetrieveFile(ctx context.Context, fileId string) ([]byte, error) {
	panic("not implemented") // TODO: Implement
}
func (s3 S3Storage) DeleteFile(ctx context.Context, fileId string) error {
	panic("not implemented") // TODO: Implement
}
