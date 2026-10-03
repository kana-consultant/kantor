package app

import (
	"context"
	"testing"

	auditrepo "github.com/kana-consultant/kantor/backend/internal/repository/audit"
)

// The scrub of existing audit_logs rows rewrites them for good, so it is
// only scheduled with the explicit AUDIT_SCRUB_EXISTING opt-in.
func TestAuditScrubJobIsOptIn(t *testing.T) {
	ctx := context.Background()
	repo := auditrepo.NewRepository(nil)

	if job := auditScrubJob(ctx, false, nil, repo); job != nil {
		t.Error("scrub scheduled although AUDIT_SCRUB_EXISTING is off")
	}
	if job := auditScrubJob(ctx, true, nil, nil); job != nil {
		t.Error("scrub scheduled without an audit repository")
	}
	if job := auditScrubJob(ctx, true, nil, repo); job == nil {
		t.Error("scrub not scheduled although AUDIT_SCRUB_EXISTING is on")
	}
}
