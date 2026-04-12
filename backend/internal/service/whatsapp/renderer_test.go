package whatsapp

import "testing"

func TestRenderTemplateReplacesAndRemovesPlaceholders(t *testing.T) {
	t.Parallel()

	body := "Halo {{name}}, buka {{app_url}}. {{unknown}}"
	got := RenderTemplate(body, map[string]string{
		"name":    "Safri",
		"app_url": "https://tenant.example.com",
	})

	want := "Halo Safri, buka https://tenant.example.com."
	if got != want {
		t.Fatalf("RenderTemplate() = %q, want %q", got, want)
	}
}

func TestBuildReviewerNotesSection(t *testing.T) {
	t.Parallel()

	if got := BuildReviewerNotesSection("  "); got != "" {
		t.Fatalf("BuildReviewerNotesSection(empty) = %q, want empty string", got)
	}

	want := "[Catatan] Sudah sesuai policy"
	if got := BuildReviewerNotesSection("Sudah sesuai policy"); got != want {
		t.Fatalf("BuildReviewerNotesSection() = %q, want %q", got, want)
	}
}

func TestSampleVarsUsesProvidedAppURL(t *testing.T) {
	t.Parallel()

	appURL := "https://kantor.perfect10.bot"
	vars := SampleVars(appURL)
	if vars["app_url"] != appURL {
		t.Fatalf("SampleVars app_url = %q, want %q", vars["app_url"], appURL)
	}
	if vars["pending_count"] == "" || vars["items_summary"] == "" {
		t.Fatal("SampleVars should include reimbursement reminder variables")
	}
}
