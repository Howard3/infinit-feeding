package student

import (
	"context"
	"fmt"
	"geevly/gen/go/eda"
	"time"

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
	Filters  StudentListFilters
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
		case *eda.Student_SetEligibility:
			return agg.SetEligibility(cmd)
		case *eda.Student_UpdateSponsorship:
			return agg.UpdateSponsorship(cmd)
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

// GetStudentEvent returns a specific event for a student Aggregate
func (s *StudentService) GetStudentEvent(ctx context.Context, studentID, eventID uint64) (*gosignal.Event, error) {
	return s.repo.getEvent(ctx, studentID, eventID)
}

// First, let's create a functional options pattern for our filters
type ListOption func(*StudentListFilters)

func ActiveOnly() ListOption {
	return func(f *StudentListFilters) {
		f.ActiveOnly = true
	}
}

func EligibleForSponsorshipOnly() ListOption {
	return func(f *StudentListFilters) {
		f.EligibleForSponsorshipOnly = true
	}
}

// Add a new filter option for school IDs
func InSchools(schoolIDs ...uint64) ListOption {
	return func(f *StudentListFilters) {
		f.SchoolIDs = schoolIDs
	}
}

// Add new filter options for age
func MinAge(age int) ListOption {
	return func(f *StudentListFilters) {
		maxBirthDate := time.Now().AddDate(-age, 0, 0)
		f.MinBirthDate = &maxBirthDate
	}
}

func MaxAge(age int) ListOption {
	return func(f *StudentListFilters) {
		minBirthDate := time.Now().AddDate(-(age + 1), 0, 1)
		f.MaxBirthDate = &minBirthDate
	}
}

// Add new filter option for name search
func WithNameSearch(search string) ListOption {
	return func(f *StudentListFilters) {
		f.NameSearch = search
	}
}

// Update the ListStudents method to use variadic options
func (s *StudentService) ListStudents(ctx context.Context, limit, page uint, opts ...ListOption) (*ListStudentsResponse, error) {
	// Create default filters
	filters := StudentListFilters{}

	// Apply any provided options
	for _, opt := range opts {
		opt(&filters)
	}

	students, err := s.repo.ListStudents(ctx, limit, page, filters)
	if err != nil {
		return nil, fmt.Errorf("failed to list students: %w", err)
	}

	count, err := s.repo.CountStudents(ctx, filters)
	if err != nil {
		return nil, fmt.Errorf("failed to count students: %w", err)
	}

	return &ListStudentsResponse{
		Students: students,
		Count:    count,
		Filters:  filters,
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

type StudentFeedingHistory struct {
	Student *ProjectedStudent
	Events  []*ProjectedFeedingEvent
}

func (s *StudentService) GetSchoolFeedingEvents(ctx context.Context, schoolID string, from, to time.Time) ([]*GroupedByStudentReturn, error) {
	q := FeedingHistoryQuery{SchoolID: schoolID, From: from, To: to}
	events, err := s.repo.QueryFeedingHistory(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("failed to query feeding history: %w", err)
	}

	return events.GroupByStudent(), nil
}

func (s *StudentService) ListForSchool(ctx context.Context, schoolID string) ([]*ProjectedStudent, error) {
	return s.repo.ListStudentsForSchool(ctx, schoolID)
}

func (s *StudentService) GetCurrentSponsorships(ctx context.Context, sponsorID string) ([]*SponsorshipProjection, error) {
	return s.repo.GetCurrentSponsorships(ctx, sponsorID)
}

func (s *StudentService) GetSponsorImpactMetrics(ctx context.Context, sponsorID string) (int64, error) {
	// Get all sponsorships for this sponsor
	sponsorships, err := s.repo.GetAllSponsorshipsByID(ctx, sponsorID)
	if err != nil {
		return 0, fmt.Errorf("failed to get sponsorships: %w", err)
	}

	var totalMeals int64
	for _, sponsorship := range sponsorships {
		// Get feeding events that occurred during this sponsorship period
		meals, err := s.repo.CountFeedingEventsInPeriod(
			ctx,
			sponsorship.StudentID,
			sponsorship.StartDate,
			sponsorship.EndDate,
		)
		if err != nil {
			return 0, fmt.Errorf("failed to count feeding events: %w", err)
		}
		totalMeals += meals
	}

	return totalMeals, nil
}

// Add this new type
type SponsorFeedingEvent struct {
	StudentID   string
	StudentName string
	FeedingTime time.Time
	SchoolID    string
}

// Add this new method
func (s *StudentService) ListSponsorFeedingEvents(ctx context.Context, sponsorID string, limit, page uint) ([]*SponsorFeedingEvent, int64, error) {
	// Get all sponsorships for this sponsor
	sponsorships, err := s.repo.GetAllSponsorshipsByID(ctx, sponsorID)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get sponsorships: %w", err)
	}

	// Get feeding events for all sponsorship periods
	events, total, err := s.repo.GetFeedingEventsForSponsorships(ctx, sponsorships, limit, page)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get feeding events: %w", err)
	}

	return events, total, nil
}

func (s *StudentService) GetAllCurrentSponsorships(ctx context.Context) ([]*SponsorshipProjection, error) {
	// Get all current sponsorships from the repository
	sponsorships, err := s.repo.GetAllCurrentSponsorships(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get current sponsorships: %w", err)
	}
	return sponsorships, nil
}
