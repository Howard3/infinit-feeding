package file

import (
	"context"
	"errors"
	"fmt"
	"geevly/gen/go/eda"

	"github.com/Howard3/gosignal"
	"github.com/oklog/ulid/v2"
)

// ErrNoFileData is returned when no file data is provided
var ErrNoFileData = errors.New("no file data provided")

// ErrFileDomainReferenceNotDefined is returned when the domain reference is not defined
var ErrFileDomainReferenceNotDefined = errors.New("file domain reference not defined")

// ErrFailedToStoreFile is returned when the file storage fails to store the file
var ErrFailedToStoreFile = errors.New("failed to store file")

type Storage interface {
	StoreFile(ctx context.Context, domainReference, fileID string, fileData []byte) error
	RetrieveFile(ctx context.Context, domainReference, fileID string) ([]byte, error)
	DeleteFile(ctx context.Context, domainReference, fileID string) error
}

type Service struct {
	repo          Repository
	eventHandlers *eventHandlers
	storage       Storage
}

func NewService(repo Repository, storage Storage) *Service {
	return &Service{
		repo:          repo,
		eventHandlers: NewEventHandlers(repo),
		storage:       storage,
	}
}

// CreateFile handles the creation of a new file, storing it, and creating an event
func (s *Service) CreateFile(ctx context.Context, fileData []byte, file *eda.File) (string, error) {
	id := ulid.Make()

	if file == nil {
		return "", ErrNoFileData
	}

	if file.DomainReference == "" {
		return "", ErrFileDomainReferenceNotDefined
	}

	if err := s.storage.StoreFile(ctx, file.DomainReference, id.String(), fileData); err != nil {
		return "", errors.Join(ErrFailedToStoreFile, err)
	}

	agg := &Aggregate{}
	agg.SetID(id.String())

	evt, err := agg.CreateFile(file)
	if err != nil {
		return "", err
	}

	if err := s.repo.saveEvent(ctx, evt); err != nil {
		return "", fmt.Errorf("failed to save event: %w", err)
	}

	return id.String(), nil
}

// DeleteFile invokes the aggregate to delete the file and removes it from storage
func (s *Service) DeleteFile(ctx context.Context, cmd *eda.File_Delete) error {
	_, err := s.withFile(ctx, func(f *Aggregate) (*gosignal.Event, error) {
		return f.DeleteFile(cmd)
	})
	if err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}

	if err := s.storage.DeleteFile(ctx, cmd.DomainReference, cmd.Id); err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}

	return nil
}

// withFile is a helper function to load, execute a command, and persist an aggregate
func (s *Service) withFile(ctx context.Context, commandFunc func(*Aggregate) (*gosignal.Event, error)) (*Aggregate, error) {
	// Load the aggregate
	f := &Aggregate{}
	// Assuming the loading and ID setting mechanism is implemented elsewhere

	// Execute the command function to perform operations and get an event
	evt, err := commandFunc(f)
	if err != nil {
		return nil, err
	}

	// Handle event saving and apply it to the aggregate if needed
	if evt != nil {
		// Persist the event
		if err := s.repo.saveEvent(ctx, evt); err != nil {
			return nil, err
		}

		// Apply the event to the aggregate to update its state
		if err := f.Apply(*evt); err != nil {
			return nil, err
		}
	}

	return f, nil
}
