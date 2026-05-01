package whatsapp

import (
	"errors"
	"strings"
	"testing"
)

func TestParseSpecificUsersTargetConfig_NormalizesAndDeduplicates(t *testing.T) {
	raw := `[
		"11111111-1111-1111-1111-111111111111",
		"11111111-1111-1111-1111-111111111111",
		"22222222-2222-2222-2222-222222222222",
		""
	]`

	ids, err := parseSpecificUsersTargetConfig(&raw)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(ids) != 2 {
		t.Fatalf("expected 2 unique ids, got %d (%v)", len(ids), ids)
	}
	if ids[0] != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("unexpected first id: %q", ids[0])
	}
	if ids[1] != "22222222-2222-2222-2222-222222222222" {
		t.Fatalf("unexpected second id: %q", ids[1])
	}
}

func TestParseSpecificUsersTargetConfig_ObjectPayload(t *testing.T) {
	raw := `{"user_ids":["33333333-3333-3333-3333-333333333333"]}`

	ids, err := parseSpecificUsersTargetConfig(&raw)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(ids) != 1 || ids[0] != "33333333-3333-3333-3333-333333333333" {
		t.Fatalf("unexpected ids: %v", ids)
	}
}

func TestParseSpecificUsersTargetConfig_InvalidUUID(t *testing.T) {
	raw := `["not-a-uuid"]`

	_, err := parseSpecificUsersTargetConfig(&raw)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "valid UUID") {
		t.Fatalf("expected UUID validation message, got %q", err.Error())
	}
}

func TestParseProjectMembersTargetConfig_ValidUUID(t *testing.T) {
	raw := "44444444-4444-4444-4444-444444444444"

	projectID, err := parseProjectMembersTargetConfig(&raw)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if projectID != "44444444-4444-4444-4444-444444444444" {
		t.Fatalf("unexpected project id: %q", projectID)
	}
}

func TestParseProjectMembersTargetConfig_JSONPayload(t *testing.T) {
	raw := `{"project_id":"55555555-5555-5555-5555-555555555555"}`

	projectID, err := parseProjectMembersTargetConfig(&raw)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if projectID != "55555555-5555-5555-5555-555555555555" {
		t.Fatalf("unexpected project id: %q", projectID)
	}
}

func TestParseProjectMembersTargetConfig_InvalidUUID(t *testing.T) {
	raw := "invalid-project-id"

	_, err := parseProjectMembersTargetConfig(&raw)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "valid project UUID") {
		t.Fatalf("expected project UUID validation message, got %q", err.Error())
	}
}

func TestValidateScheduleTargetConfig_WrapsValidationError(t *testing.T) {
	raw := `["not-a-uuid"]`

	err := validateScheduleTargetConfig("specific_users", &raw)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrInvalidScheduleTargetConfig) {
		t.Fatalf("expected ErrInvalidScheduleTargetConfig, got %v", err)
	}
}

func TestValidateScheduleTargetConfig_UnsupportedType(t *testing.T) {
	err := validateScheduleTargetConfig("unknown_target", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrInvalidScheduleTargetConfig) {
		t.Fatalf("expected ErrInvalidScheduleTargetConfig, got %v", err)
	}
}
