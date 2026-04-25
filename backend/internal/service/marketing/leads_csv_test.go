package marketing

import (
	"reflect"
	"sort"
	"testing"
)

func TestBuildLeadCSVHeaderIndex_AllHeadersPresent(t *testing.T) {
	header := []string{
		"Name", "Phone", "Email",
		"Source Channel", "Pipeline-Status",
		"assigned_to", "notes", "Company Name", "Estimated_Value",
	}
	index, missing := buildLeadCSVHeaderIndex(header)
	if len(missing) != 0 {
		t.Fatalf("expected no missing headers, got %v", missing)
	}

	want := map[string]int{
		"name":            0,
		"phone":           1,
		"email":           2,
		"source_channel":  3,
		"pipeline_status": 4,
		"assigned_to":     5,
		"notes":           6,
		"company_name":    7,
		"estimated_value": 8,
	}
	if !reflect.DeepEqual(index, want) {
		t.Fatalf("index mismatch:\n got: %v\nwant: %v", index, want)
	}
}

func TestBuildLeadCSVHeaderIndex_ReorderedColumns(t *testing.T) {
	// Pipeline export tools often reorder columns. The importer must follow
	// the headers, not column position.
	header := []string{"email", "name", "source_channel", "pipeline_status", "phone"}
	index, missing := buildLeadCSVHeaderIndex(header)
	if len(missing) != 0 {
		t.Fatalf("expected no missing headers, got %v", missing)
	}
	if index["name"] != 1 {
		t.Fatalf("name should be at column 1, got %d", index["name"])
	}
	if index["email"] != 0 {
		t.Fatalf("email should be at column 0, got %d", index["email"])
	}
}

func TestBuildLeadCSVHeaderIndex_MissingRequired(t *testing.T) {
	header := []string{"name", "phone"}
	_, missing := buildLeadCSVHeaderIndex(header)
	sort.Strings(missing)
	want := []string{"pipeline_status", "source_channel"}
	if !reflect.DeepEqual(missing, want) {
		t.Fatalf("missing mismatch:\n got: %v\nwant: %v", missing, want)
	}
}

func TestBuildLeadCSVHeaderIndex_UnknownColumnsIgnored(t *testing.T) {
	header := []string{"name", "source_channel", "pipeline_status", "internal_score"}
	index, missing := buildLeadCSVHeaderIndex(header)
	if len(missing) != 0 {
		t.Fatalf("expected no missing headers, got %v", missing)
	}
	if _, ok := index["internal_score"]; ok {
		t.Fatalf("unknown column internal_score should be ignored")
	}
}

func TestParseLeadCSVRow_MapsByHeaderName(t *testing.T) {
	// Importing a CSV where columns are reordered relative to the legacy
	// positional schema must still yield the correct fields.
	header := []string{"email", "name", "source_channel", "pipeline_status", "phone", "estimated_value"}
	index, _ := buildLeadCSVHeaderIndex(header)
	row := []string{"a@b.test", "Alice", "wa", "new", "+62811", "150000"}

	got, err := parseLeadCSVRow(row, index)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if got.Name != "Alice" {
		t.Fatalf("expected Name=Alice, got %q", got.Name)
	}
	if got.SourceChannel != "wa" {
		t.Fatalf("expected SourceChannel=wa, got %q", got.SourceChannel)
	}
	if got.PipelineStatus != "new" {
		t.Fatalf("expected PipelineStatus=new, got %q", got.PipelineStatus)
	}
	if got.Phone == nil || *got.Phone != "+62811" {
		t.Fatalf("expected Phone=+62811, got %v", got.Phone)
	}
	if got.Email == nil || *got.Email != "a@b.test" {
		t.Fatalf("expected Email=a@b.test, got %v", got.Email)
	}
	if got.EstimatedValue != 150000 {
		t.Fatalf("expected EstimatedValue=150000, got %d", got.EstimatedValue)
	}
}

func TestParseLeadCSVRow_InvalidEstimatedValue(t *testing.T) {
	header := []string{"name", "source_channel", "pipeline_status", "estimated_value"}
	index, _ := buildLeadCSVHeaderIndex(header)
	row := []string{"Alice", "wa", "new", "not-a-number"}

	if _, err := parseLeadCSVRow(row, index); err == nil {
		t.Fatalf("expected error for non-numeric estimated_value, got nil")
	}
}
