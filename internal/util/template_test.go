package util

import (
	"strings"
	"testing"
)

func TestRenderTemplate_Basic(t *testing.T) {
	tmpl := "Hello {{.Name}}, you are {{.Age}} years old."
	data := map[string]interface{}{
		"Name": "Alice",
		"Age":  30,
	}

	result, err := RenderTemplate(tmpl, data)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	expected := "Hello Alice, you are 30 years old."
	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}

func TestRenderTemplate_ComplexData(t *testing.T) {
	tmpl := "Main topic: {{.MainTopic}}\nGenerate {{.NumSubtopics}} subtopics."
	data := map[string]interface{}{
		"MainTopic":    "Fantasy Fiction",
		"NumSubtopics": 5,
	}

	result, err := RenderTemplate(tmpl, data)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if !strings.Contains(result, "Fantasy Fiction") {
		t.Errorf("Result should contain 'Fantasy Fiction': %s", result)
	}
	if !strings.Contains(result, "5 subtopics") {
		t.Errorf("Result should contain '5 subtopics': %s", result)
	}
}

func TestRenderTemplate_InvalidTemplate(t *testing.T) {
	tmpl := "Hello {{.Name" // Missing closing braces
	data := map[string]interface{}{
		"Name": "Alice",
	}

	_, err := RenderTemplate(tmpl, data)
	if err == nil {
		t.Error("Expected error for invalid template, got nil")
	}
}

func TestRenderTemplate_MissingData(t *testing.T) {
	tmpl := "Hello {{.Name}}"
	data := map[string]interface{}{} // Empty data

	result, err := RenderTemplate(tmpl, data)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Template should render with empty value
	expected := "Hello <no value>"
	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}

func TestRenderTemplate_EmptyTemplate(t *testing.T) {
	tmpl := ""
	data := map[string]interface{}{"Name": "Alice"}

	result, err := RenderTemplate(tmpl, data)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if result != "" {
		t.Errorf("Expected empty result, got '%s'", result)
	}
}
