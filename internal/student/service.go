package student

import (
	"context"
	"fmt"
	"geevly/gen/go/eda"

	"github.com/Howard3/gosignal"
	"google.golang.org/protobuf/proto"
)

var ErrSchoolValidation = fmt.Errorf("error validating school")

type StudentService struct {
	repo          Repository
	eventHandlers *eventHandlers
	acl           AntiCorruptionLayer
}

type AntiCorruptionLayer interface {
	ValidateSchoolID(ctx context.Context, schoolID string) error
	ValidatePhotoID(ctx context.Context, photoID string) error
}

func NewStudentService(repo Repository, acl AntiCorruptionLayer) *StudentService {
	return &StudentService{
		repo:          repo,
		eventHandlers: NewEventHandlers(repo),
		acl:           acl,
	}
}

type ListStudentsResponse struct {
	Students []*ProjectedStudent
	Count    uint
}

// RunCommand runs a command on a user aggregate
func (s *StudentService) RunCommand(ctx context.Context, aggID uint64, cmd proto.Message) (*Aggregate, error) {
	return s.withAgg(ctx, aggID, func(agg *Aggregate) (*gosignal.Event, error) {
		switch cmd := cmd.(type) {
		case *eda.Student_Feeding:
			return agg.Feed(cmd)
		case *eda.Student_Enroll:
			if err := s.acl.ValidateSchoolID(ctx, cmd.GetSchoolId()); err != nil {
				return nil, fmt.Errorf("failed to validate school ID: %w", err)
			}
			return agg.EnrollStudent(cmd)
		case *eda.Student_SetLookupCode:
			// TODO: check for collisions on the lookup code
			return agg.SetLookupCode(cmd)
		case *eda.Student_Unenroll:
			return agg.UnenrollStudent(cmd)
		case *eda.Student_Update:
			return agg.UpdateStudent(cmd)
		case *eda.Student_SetStatus:
			return agg.SetStatus(cmd)
		case *eda.Student_SetProfilePhoto:
			if err := s.acl.ValidatePhotoID(ctx, cmd.GetFileId()); err != nil {
				return nil, fmt.Errorf("failed to validate photo ID: %w", err)
			}
			return agg.SetProfilePhoto(cmd)
		default:
			return nil, fmt.Errorf("unknown command type: %T", cmd)
		}
	})
}

// withUser is a helper function that loads an user aggregate from the repository and executes a function on it
func (s *StudentService) withAgg(ctx context.Context, id uint64, fn func(*Aggregate) (*gosignal.Event, error)) (*Aggregate, error) {
	agg, err := s.repo.loadStudent(ctx, id)
	if err != nil {
		return nil, err
	}

	evt, err := fn(agg)
	if err != nil {
		return nil, err
	}

	return agg, s.saveEvent(ctx, evt)
}

func (s *StudentService) saveEvent(ctx context.Context, evt *gosignal.Event) error {
	if evt != nil {
		if err := s.repo.saveEvents(ctx, []gosignal.Event{*evt}); err != nil {
			return err
		}

		go s.eventHandlers.routeEvent(context.Background(), evt)

	}

	return nil
}

func (s *StudentService) ListStudents(ctx context.Context, limit, page uint) (*ListStudentsResponse, error) {
	students, err := s.repo.ListStudents(ctx, limit, page)
	if err != nil {
		return nil, fmt.Errorf("failed to list students: %w", err)
	}

	count, err := s.repo.CountStudents(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to count students: %w", err)
	}

	return &ListStudentsResponse{
		Students: students,
		Count:    count,
	}, nil
}
func (s *StudentService) CreateStudent(ctx context.Context, req *eda.Student_Create) (*Aggregate, error) {
	studentAgg := &Aggregate{}
	newID, err := s.repo.GetNewID(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get new ID: %w", err)
	}

	studentAgg.SetIDUint64(newID)

	evt, err := studentAgg.CreateStudent(req)
	if err != nil {
		return nil, err
	}

	return studentAgg, s.saveEvent(ctx, evt)
}

// GetStudent returns a student aggregate by ID
func (s *StudentService) GetStudent(ctx context.Context, studentID uint64) (*Aggregate, error) {
	studentAgg, err := s.repo.loadStudent(ctx, studentID)
	if err != nil {
		return nil, err
	}

	return studentAgg, nil
}

// GetHistory returns the event history for a student aggregate
func (s *StudentService) GetHistory(ctx context.Context, studentID uint64) ([]gosignal.Event, error) {
	return s.repo.getEventHistory(ctx, studentID)
}

// GetStudentByCode returns a student by a lookup code
func (s *StudentService) GetStudentByCode(ctx context.Context, code []byte) (*Aggregate, error) {
	id, err := s.repo.getStudentIDByCode(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("failed to get student ID by code: %w", err)
	}

	return s.GetStudent(ctx, id)
}

// GetStudentByStudentSchoolID returns a student by a student school ID
func (s *StudentService) GetStudentByStudentSchoolID(ctx context.Context, studentSchoolID string) (*Aggregate, error) {
	id, err := s.repo.getStudentIDByStudentSchoolID(ctx, studentSchoolID)
	if err != nil {
		return nil, fmt.Errorf("failed to get student ID by student school ID: %w", err)
	}

	return s.GetStudent(ctx, id)
}
