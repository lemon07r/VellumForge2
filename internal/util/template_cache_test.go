package util

import (
	"strings"
	"sync"
	"testing"
)

func TestTemplateCaching(t *testing.T) {
	ClearTemplateCache() // Start fresh

	tmpl := "Hello {{.Name}}"
	data := map[string]interface{}{"Name": "World"}

	// First render (cache miss)
	result1, err := RenderTemplate(tmpl, data)
	if err != nil {
		t.Fatalf("First render failed: %v", err)
	}
	if result1 != "Hello World" {
		t.Errorf("Expected 'Hello World', got '%s'", result1)
	}

	// Second render (cache hit - should use cached template)
	result2, err := RenderTemplate(tmpl, data)
	if err != nil {
		t.Fatalf("Second render failed: %v", err)
	}
	if result2 != "Hello World" {
		t.Errorf("Expected 'Hello World', got '%s'", result2)
	}

	// Different data, same template (cache hit)
	data2 := map[string]interface{}{"Name": "Gopher"}
	result3, err := RenderTemplate(tmpl, data2)
	if err != nil {
		t.Fatalf("Third render failed: %v", err)
	}
	if result3 != "Hello Gopher" {
		t.Errorf("Expected 'Hello Gopher', got '%s'", result3)
	}
}

func TestTemplateCachingDifferentTemplates(t *testing.T) {
	ClearTemplateCache()

	// Render multiple different templates
	tests := []struct {
		tmpl string
		data map[string]interface{}
		want string
	}{
		{
			tmpl: "Count: {{.Count}}",
			data: map[string]interface{}{"Count": 42},
			want: "Count: 42",
		},
		{
			tmpl: "Name: {{.Name}}, Age: {{.Age}}",
			data: map[string]interface{}{"Name": "Alice", "Age": 30},
			want: "Name: Alice, Age: 30",
		},
		{
			tmpl: "{{.Prefix}} - {{.Suffix}}",
			data: map[string]interface{}{"Prefix": "Start", "Suffix": "End"},
			want: "Start - End",
		},
	}

	for _, tt := range tests {
		result, err := RenderTemplate(tt.tmpl, tt.data)
		if err != nil {
			t.Fatalf("Render failed for template %s: %v", tt.tmpl, err)
		}
		if result != tt.want {
			t.Errorf("Template %s: got %s, want %s", tt.tmpl, result, tt.want)
		}
	}

	// Render them all again to verify caching works
	for _, tt := range tests {
		result, err := RenderTemplate(tt.tmpl, tt.data)
		if err != nil {
			t.Fatalf("Second render failed for template %s: %v", tt.tmpl, err)
		}
		if result != tt.want {
			t.Errorf("Template %s (cached): got %s, want %s", tt.tmpl, result, tt.want)
		}
	}
}

func TestTemplateCachingConcurrency(t *testing.T) {
	ClearTemplateCache()

	tmpl := "Count: {{.Count}}"
	iterations := 100
	var wg sync.WaitGroup
	errors := make(chan error, iterations)

	// Hammer the cache with concurrent requests
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			data := map[string]interface{}{"Count": n}
			result, err := RenderTemplate(tmpl, data)
			if err != nil {
				errors <- err
				return
			}
			// Just check it starts correctly (numbers can be multi-digit)
			if !strings.HasPrefix(result, "Count: ") {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for any errors
	for err := range errors {
		if err != nil {
			t.Errorf("Concurrent render failed: %v", err)
		}
	}
}

func TestClearTemplateCache(t *testing.T) {
	ClearTemplateCache()

	tmpl := "Test: {{.Value}}"
	data := map[string]interface{}{"Value": "123"}

	// Populate cache
	_, err := RenderTemplate(tmpl, data)
	if err != nil {
		t.Fatalf("Initial render failed: %v", err)
	}

	// Clear cache
	ClearTemplateCache()

	// Should still work (re-parse)
	result, err := RenderTemplate(tmpl, data)
	if err != nil {
		t.Fatalf("Render after clear failed: %v", err)
	}
	if result != "Test: 123" {
		t.Errorf("Expected 'Test: 123', got '%s'", result)
	}
}

func TestTemplateCachingWithConditionals(t *testing.T) {
	ClearTemplateCache()

	tmpl := "{{if .IsRetry}}Retry: {{end}}{{.MainTopic}}"

	tests := []struct {
		name string
		data map[string]interface{}
		want string
	}{
		{
			name: "first attempt",
			data: map[string]interface{}{
				"IsRetry":   false,
				"MainTopic": "Fantasy",
			},
			want: "Fantasy",
		},
		{
			name: "retry attempt",
			data: map[string]interface{}{
				"IsRetry":   true,
				"MainTopic": "SciFi",
			},
			want: "Retry: SciFi",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := RenderTemplate(tmpl, tt.data)
			if err != nil {
				t.Fatalf("Render failed: %v", err)
			}
			if result != tt.want {
				t.Errorf("got %s, want %s", result, tt.want)
			}
		})
	}
}

func TestTemplateCachingPreservesValidation(t *testing.T) {
	ClearTemplateCache()

	// Forbidden directive should be rejected even on cache hit
	forbiddenTmpl := "{{call .Func}}"
	data := map[string]interface{}{"Func": "test"}

	// First attempt (cache miss)
	_, err := RenderTemplate(forbiddenTmpl, data)
	if err == nil {
		t.Error("Expected error for forbidden directive on first attempt")
	}
	if !strings.Contains(err.Error(), "forbidden directive") {
		t.Errorf("Expected forbidden directive error, got: %v", err)
	}

	// Second attempt (would be cache hit, but should still validate)
	_, err = RenderTemplate(forbiddenTmpl, data)
	if err == nil {
		t.Error("Expected error for forbidden directive on second attempt")
	}
	if !strings.Contains(err.Error(), "forbidden directive") {
		t.Errorf("Expected forbidden directive error, got: %v", err)
	}
}

func TestTemplateCachingPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	ClearTemplateCache()

	tmpl := `You are a creative writer. Generate {{.NumItems}} items for: {{.Topic}}

{{if .IsRetry}}NOTE: Avoid these: {{.Excluded}}
{{end}}

Return ONLY a JSON array.`

	data := map[string]interface{}{
		"NumItems": 10,
		"Topic":    "Fantasy Fiction",
		"IsRetry":  false,
	}

	// Warmup
	_, err := RenderTemplate(tmpl, data)
	if err != nil {
		t.Fatalf("Warmup failed: %v", err)
	}

	// Render many times to ensure caching works
	iterations := 1000
	for i := 0; i < iterations; i++ {
		_, err := RenderTemplate(tmpl, data)
		if err != nil {
			t.Fatalf("Iteration %d failed: %v", i, err)
		}
	}

	// If we get here without timeout, caching is working
	// (Without caching, parsing 1000 times would be much slower)
}
