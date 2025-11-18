package webapi

import (
	"encoding/json"
	"fmt"
	"geevly/gen/go/eda"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// FormatEventData attempts to deserialize and format event data as human-readable JSON
func FormatEventData(domain, eventType string, data []byte) string {
	if len(data) == 0 {
		return "{}"
	}

	var msg proto.Message

	// Map event types to their protobuf message types
	switch domain {
	case "student":
		msg = getStudentEventMessage(eventType)
	case "school":
		msg = getSchoolEventMessage(eventType)
	case "file":
		msg = getFileEventMessage(eventType)
	case "bulk_upload":
		msg = getBulkUploadEventMessage(eventType)
	default:
		return formatAsHex(data)
	}

	if msg == nil {
		return formatAsHex(data)
	}

	// Unmarshal the protobuf data
	if err := proto.Unmarshal(data, msg); err != nil {
		return fmt.Sprintf("Error deserializing: %v\n\nHex: %x", err, data)
	}

	// Convert to JSON for readable display
	jsonBytes, err := protojson.MarshalOptions{
		Multiline:       true,
		Indent:          "  ",
		EmitUnpopulated: false,
	}.Marshal(msg)

	if err != nil {
		return fmt.Sprintf("Error converting to JSON: %v", err)
	}

	// Pretty print the JSON
	var prettyJSON map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &prettyJSON); err == nil {
		if formatted, err := json.MarshalIndent(prettyJSON, "", "  "); err == nil {
			return string(formatted)
		}
	}

	return string(jsonBytes)
}

func getStudentEventMessage(eventType string) proto.Message {
	switch eventType {
	case "AddStudent":
		return &eda.Student_Create_Event{}
	case "UpdateStudent":
		return &eda.Student_Update_Event{}
	case "FeedStudent":
		return &eda.Student_Feeding_Event{}
	case "AddHealthAssessment":
		return &eda.Student_HealthAssessment_Event{}
	case "RemoveHealthAssessment":
		return &eda.Student_HealthAssessment_UndoEvent{}
	case "AddGradeReport":
		return &eda.Student_GradeReport_Event{}
	case "RemoveGradeReport":
		return &eda.Student_GradeReport_UndoEvent{}
	case "SetStudentStatus":
		return &eda.Student_SetStatus_Event{}
	case "EnrollStudent":
		return &eda.Student_Enroll_Event{}
	case "UnenrollStudent":
		return &eda.Student_Unenroll_Event{}
	case "SetLookupCode":
		return &eda.Student_SetLookupCode_Event{}
	case "SetProfilePhoto":
		return &eda.Student_SetProfilePhoto_Event{}
	case "UpdateSponsorship":
		return &eda.Student_UpdateSponsorship_Event{}
	case "SetEligibility":
		return &eda.Student_SetEligibility_Event{}
	default:
		return nil
	}
}

func getSchoolEventMessage(eventType string) proto.Message {
	switch eventType {
	case "CreateSchool":
		return &eda.School_Create_Event{}
	case "UpdateSchool":
		return &eda.School_Update_Event{}
	case "SetSchoolPeriod":
		return &eda.School_SetSchoolPeriod_Event{}
	default:
		return nil
	}
}

func getFileEventMessage(eventType string) proto.Message {
	switch eventType {
	case "FileCreated":
		return &eda.File_Create_Event{}
	case "FileDeleted":
		return &eda.File_Delete_Event{}
	default:
		return nil
	}
}

func getBulkUploadEventMessage(eventType string) proto.Message {
	switch eventType {
	case "BulkUploadCreate":
		return &eda.BulkUpload_Create_Event{}
	case "BulkUploadAddValidationErrors":
		return &eda.BulkUpload_ValidationError_Event{}
	case "BulkUploadSetStatus":
		// This might be embedded in another message - check if needed
		return nil
	case "BulkUploadRecordActionsEvent":
		// This might be embedded in another message - check if needed
		return nil
	default:
		return nil
	}
}

func formatAsHex(data []byte) string {
	return fmt.Sprintf("Unable to deserialize - showing hex:\n\n%x", data)
}
