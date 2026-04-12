package hris

import (
	"errors"
	"reflect"
	"testing"
)

func TestFilterKeptAttachments(t *testing.T) {
	t.Parallel()

	existing := []string{
		"reimbursements/a.png",
		"reimbursements/b.png",
	}

	kept, err := filterKeptAttachments(existing, []string{
		" reimbursements/a.png ",
		"reimbursements/a.png",
	})
	if err != nil {
		t.Fatalf("filterKeptAttachments returned error: %v", err)
	}

	want := []string{"reimbursements/a.png"}
	if !reflect.DeepEqual(kept, want) {
		t.Fatalf("filterKeptAttachments() = %#v, want %#v", kept, want)
	}
}

func TestFilterKeptAttachmentsRejectsUnknownPath(t *testing.T) {
	t.Parallel()

	_, err := filterKeptAttachments([]string{"reimbursements/a.png"}, []string{"reimbursements/evil.png"})
	if !errors.Is(err, ErrReimbursementInvalidAttachment) {
		t.Fatalf("filterKeptAttachments error = %v, want %v", err, ErrReimbursementInvalidAttachment)
	}
}

func TestDiffAttachmentPaths(t *testing.T) {
	t.Parallel()

	removed := diffAttachmentPaths(
		[]string{"reimbursements/a.png", "reimbursements/b.png"},
		[]string{"reimbursements/b.png"},
	)

	want := []string{"reimbursements/a.png"}
	if !reflect.DeepEqual(removed, want) {
		t.Fatalf("diffAttachmentPaths() = %#v, want %#v", removed, want)
	}
}

func TestUniqueIDs(t *testing.T) {
	t.Parallel()

	got := uniqueIDs([]string{" user-1 ", "", "user-2", "user-1"})
	want := []string{"user-1", "user-2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("uniqueIDs() = %#v, want %#v", got, want)
	}
}
