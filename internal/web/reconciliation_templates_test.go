package web

import (
	"strings"
	"testing"
)

func readTemplateForTest(t *testing.T, name string) string {
	t.Helper()
	raw, err := content.ReadFile("templates/" + name)
	if err != nil {
		t.Fatalf("failed to read template %s: %v", name, err)
	}
	return string(raw)
}

func TestIndexTemplateDoesNotExposeReconciliationUI(t *testing.T) {
	index := readTemplateForTest(t, "index.html")
	forbidden := []string{
		"toggleReconciliationBtn",
		"reconciliationContainer",
		"reconciliationForm",
		"reconciliationTargetAmount",
		"renderReconciliationPreview(",
		"toggleReconciliationPanel(",
		"Ajuste conciliacion CA",
		"tags: ['conciliacion', 'ajuste']",
	}

	for _, token := range forbidden {
		if strings.Contains(index, token) {
			t.Fatalf("index.html should not contain reconciliation token %q", token)
		}
	}
}

func TestSettingsTemplateKeepsReconciliationModule(t *testing.T) {
	settings := readTemplateForTest(t, "settings.html")
	required := []string{
		"openReconciliationSection",
		"settingsReconciliationForm",
		"/reconciliation/apply",
		"/reconciliation/history",
		"/reconciliation/revert",
	}

	for _, token := range required {
		if !strings.Contains(settings, token) {
			t.Fatalf("settings.html should contain reconciliation token %q", token)
		}
	}
}
