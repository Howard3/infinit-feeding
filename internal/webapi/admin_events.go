package webapi

import (
	"context"
	"fmt"
	"geevly/internal/bulk_upload"
	"geevly/internal/file"
	"geevly/internal/school"
	"geevly/internal/student"
	reportstempl "geevly/internal/webapi/templates/admin/reports"
	components "geevly/internal/webapi/templates/components"
	"net/http"
	"time"
)

// adminEventsViewer renders the event stream viewer page
func (s *Server) adminEventsViewer(w http.ResponseWriter, r *http.Request) {
	domain := r.URL.Query().Get("domain")
	if domain == "" {
		domain = "student" // Default to student domain
	}

	eventTypeFilter := r.URL.Query().Get("event_type")
	aggregateIDFilter := r.URL.Query().Get("aggregate_id")

	// Parse date range filters
	var startDate, endDate *time.Time
	if sd := r.URL.Query().Get("start_date"); sd != "" {
		if parsed, err := time.Parse(time.RFC3339, sd); err == nil {
			startDate = &parsed
		}
	}
	if ed := r.URL.Query().Get("end_date"); ed != "" {
		if parsed, err := time.Parse(time.RFC3339, ed); err == nil {
			endDate = &parsed
		}
	}

	page := s.pageQuery(r)
	limit := uint(50) // Show 50 events per page

	var events []reportstempl.DomainEvent
	var eventTypesList []string
	var stats *reportstempl.EventStatistics
	var total uint
	var err error

	ctx := r.Context()

	// Get events, event types, and statistics from the appropriate domain service
	switch domain {
	case "student":
		events, total, err = s.getStudentEvents(ctx, limit, page, eventTypeFilter, aggregateIDFilter, startDate, endDate)
		if err == nil {
			eventTypesList, err = s.Services.StudentSvc.GetEventTypes(ctx)
		}
		if err == nil {
			var studentStats *student.EventStatistics
			studentStats, err = s.Services.StudentSvc.GetEventStatistics(ctx)
			if err == nil {
				stats = convertStudentStats(studentStats)
			}
		}
	case "school":
		events, total, err = s.getSchoolEvents(ctx, limit, page, eventTypeFilter, aggregateIDFilter, startDate, endDate)
		if err == nil {
			eventTypesList, err = s.Services.SchoolSvc.GetEventTypes(ctx)
		}
		if err == nil {
			var schoolStats *school.EventStatistics
			schoolStats, err = s.Services.SchoolSvc.GetEventStatistics(ctx)
			if err == nil {
				stats = convertSchoolStats(schoolStats)
			}
		}
	case "file":
		events, total, err = s.getFileEvents(ctx, limit, page, eventTypeFilter, aggregateIDFilter, startDate, endDate)
		if err == nil {
			eventTypesList, err = s.Services.FileSvc.GetEventTypes(ctx)
		}
		if err == nil {
			var fileStats *file.EventStatistics
			fileStats, err = s.Services.FileSvc.GetEventStatistics(ctx)
			if err == nil {
				stats = convertFileStats(fileStats)
			}
		}
	case "bulk_upload":
		events, total, err = s.getBulkUploadEvents(ctx, limit, page, eventTypeFilter, aggregateIDFilter, startDate, endDate)
		if err == nil {
			eventTypesList, err = s.Services.BulkUploadSvc.GetEventTypes(ctx)
		}
		if err == nil {
			var bulkStats *bulk_upload.EventStatistics
			bulkStats, err = s.Services.BulkUploadSvc.GetEventStatistics(ctx)
			if err == nil {
				stats = convertBulkUploadStats(bulkStats)
			}
		}
	default:
		s.errorPage(w, r, "Invalid domain", fmt.Errorf("unknown domain: %s", domain))
		return
	}

	if err != nil {
		s.errorPage(w, r, "Error fetching events", err)
		return
	}

	// Convert event types slice to map for dropdown
	eventTypes := make(map[string]string)
	for _, et := range eventTypesList {
		eventTypes[et] = et
	}

	// Build pagination URL with all active filters
	pagination := components.NewPagination(page, limit, total)
	pagination.URL = s.buildEventsURL(domain, eventTypeFilter, aggregateIDFilter, startDate, endDate)

	s.renderTempl(w, r, reportstempl.EventsViewer(domain, events, eventTypes, eventTypeFilter, aggregateIDFilter, stats, pagination))
}

// buildEventsURL constructs the pagination URL with all active filters
func (s *Server) buildEventsURL(domain, eventTypeFilter, aggregateIDFilter string, startDate, endDate *time.Time) string {
	url := fmt.Sprintf("/admin/reports/events?domain=%s", domain)
	if eventTypeFilter != "" {
		url += fmt.Sprintf("&event_type=%s", eventTypeFilter)
	}
	if aggregateIDFilter != "" {
		url += fmt.Sprintf("&aggregate_id=%s", aggregateIDFilter)
	}
	if startDate != nil {
		url += fmt.Sprintf("&start_date=%s", startDate.Format(time.RFC3339))
	}
	if endDate != nil {
		url += fmt.Sprintf("&end_date=%s", endDate.Format(time.RFC3339))
	}
	return url
}

// getStudentEvents retrieves student domain events with pagination
func (s *Server) getStudentEvents(ctx context.Context, limit, page uint, eventTypeFilter, aggregateIDFilter string, startDate, endDate *time.Time) ([]reportstempl.DomainEvent, uint, error) {
	offset := (page - 1) * limit
	domainEvents, total, err := s.Services.StudentSvc.GetDomainEvents(ctx, limit, offset, eventTypeFilter, aggregateIDFilter, startDate, endDate)
	if err != nil {
		return nil, 0, err
	}
	return convertStudentEvents(domainEvents), total, nil
}

// getSchoolEvents retrieves school domain events with pagination
func (s *Server) getSchoolEvents(ctx context.Context, limit, page uint, eventTypeFilter, aggregateIDFilter string, startDate, endDate *time.Time) ([]reportstempl.DomainEvent, uint, error) {
	offset := (page - 1) * limit
	domainEvents, total, err := s.Services.SchoolSvc.GetDomainEvents(ctx, limit, offset, eventTypeFilter, aggregateIDFilter, startDate, endDate)
	if err != nil {
		return nil, 0, err
	}
	return convertSchoolEvents(domainEvents), total, nil
}

// getFileEvents retrieves file domain events with pagination
func (s *Server) getFileEvents(ctx context.Context, limit, page uint, eventTypeFilter, aggregateIDFilter string, startDate, endDate *time.Time) ([]reportstempl.DomainEvent, uint, error) {
	offset := (page - 1) * limit
	domainEvents, total, err := s.Services.FileSvc.GetDomainEvents(ctx, limit, offset, eventTypeFilter, aggregateIDFilter, startDate, endDate)
	if err != nil {
		return nil, 0, err
	}
	return convertFileEvents(domainEvents), total, nil
}

// getBulkUploadEvents retrieves bulk_upload domain events with pagination
func (s *Server) getBulkUploadEvents(ctx context.Context, limit, page uint, eventTypeFilter, aggregateIDFilter string, startDate, endDate *time.Time) ([]reportstempl.DomainEvent, uint, error) {
	offset := (page - 1) * limit
	domainEvents, total, err := s.Services.BulkUploadSvc.GetDomainEvents(ctx, limit, offset, eventTypeFilter, aggregateIDFilter, startDate, endDate)
	if err != nil {
		return nil, 0, err
	}
	return convertBulkUploadEvents(domainEvents), total, nil
}

// Converter functions to map domain events to template events
func convertStudentEvents(events []student.DomainEvent) []reportstempl.DomainEvent {
	result := make([]reportstempl.DomainEvent, len(events))
	for i, evt := range events {
		result[i] = reportstempl.DomainEvent{
			Domain:        "student",
			Type:          evt.Type,
			AggregateID:   evt.AggregateID,
			Version:       evt.Version,
			Timestamp:     evt.Timestamp,
			Data:          evt.Data,
			FormattedData: FormatEventData("student", evt.Type, evt.Data),
		}
	}
	return result
}

func convertSchoolEvents(events []school.DomainEvent) []reportstempl.DomainEvent {
	result := make([]reportstempl.DomainEvent, len(events))
	for i, evt := range events {
		result[i] = reportstempl.DomainEvent{
			Domain:        "school",
			Type:          evt.Type,
			AggregateID:   evt.AggregateID,
			Version:       evt.Version,
			Timestamp:     evt.Timestamp,
			Data:          evt.Data,
			FormattedData: FormatEventData("school", evt.Type, evt.Data),
		}
	}
	return result
}

func convertFileEvents(events []file.DomainEvent) []reportstempl.DomainEvent {
	result := make([]reportstempl.DomainEvent, len(events))
	for i, evt := range events {
		result[i] = reportstempl.DomainEvent{
			Domain:        "file",
			Type:          evt.Type,
			AggregateID:   evt.AggregateID,
			Version:       evt.Version,
			Timestamp:     evt.Timestamp,
			Data:          evt.Data,
			FormattedData: FormatEventData("file", evt.Type, evt.Data),
		}
	}
	return result
}

func convertBulkUploadEvents(events []bulk_upload.DomainEvent) []reportstempl.DomainEvent {
	result := make([]reportstempl.DomainEvent, len(events))
	for i, evt := range events {
		result[i] = reportstempl.DomainEvent{
			Domain:        "bulk_upload",
			Type:          evt.Type,
			AggregateID:   evt.AggregateID,
			Version:       evt.Version,
			Timestamp:     evt.Timestamp,
			Data:          evt.Data,
			FormattedData: FormatEventData("bulk_upload", evt.Type, evt.Data),
		}
	}
	return result
}

// Converter functions for statistics
func convertStudentStats(stats *student.EventStatistics) *reportstempl.EventStatistics {
	return &reportstempl.EventStatistics{
		TotalEvents:      stats.TotalEvents,
		EventsByType:     stats.EventsByType,
		OldestEventTime:  stats.OldestEventTime,
		NewestEventTime:  stats.NewestEventTime,
		UniqueAggregates: stats.UniqueAggregates,
	}
}

func convertSchoolStats(stats *school.EventStatistics) *reportstempl.EventStatistics {
	return &reportstempl.EventStatistics{
		TotalEvents:      stats.TotalEvents,
		EventsByType:     stats.EventsByType,
		OldestEventTime:  stats.OldestEventTime,
		NewestEventTime:  stats.NewestEventTime,
		UniqueAggregates: stats.UniqueAggregates,
	}
}

func convertFileStats(stats *file.EventStatistics) *reportstempl.EventStatistics {
	return &reportstempl.EventStatistics{
		TotalEvents:      stats.TotalEvents,
		EventsByType:     stats.EventsByType,
		OldestEventTime:  stats.OldestEventTime,
		NewestEventTime:  stats.NewestEventTime,
		UniqueAggregates: stats.UniqueAggregates,
	}
}

func convertBulkUploadStats(stats *bulk_upload.EventStatistics) *reportstempl.EventStatistics {
	return &reportstempl.EventStatistics{
		TotalEvents:      stats.TotalEvents,
		EventsByType:     stats.EventsByType,
		OldestEventTime:  stats.OldestEventTime,
		NewestEventTime:  stats.NewestEventTime,
		UniqueAggregates: stats.UniqueAggregates,
	}
}
