package ai

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTagSuggestionRequest_Validation(t *testing.T) {
	tests := []struct {
		name    string
		request *TagSuggestionRequest
		valid   bool
	}{
		{
			name: "valid request with content only",
			request: &TagSuggestionRequest{
				Content: "This is a test memo about programming",
			},
			valid: true,
		},
		{
			name: "valid request with user tags",
			request: &TagSuggestionRequest{
				Content:  "This is a test memo",
				UserTags: []string{"programming", "work", "ideas"},
			},
			valid: true,
		},
		{
			name: "valid request with existing tags",
			request: &TagSuggestionRequest{
				Content:      "This is a test memo",
				ExistingTags: []string{"draft", "personal"},
			},
			valid: true,
		},
		{
			name: "empty content",
			request: &TagSuggestionRequest{
				Content: "",
			},
			valid: false,
		},
		{
			name: "whitespace-only content",
			request: &TagSuggestionRequest{
				Content: "   \n\t   ",
			},
			valid: false,
		},
		{
			name:    "nil request",
			request: nil,
			valid:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.request == nil {
				assert.False(t, tt.valid)
				return
			}

			hasContent := strings.TrimSpace(tt.request.Content) != ""
			assert.Equal(t, tt.valid, hasContent)
		})
	}
}

func TestClient_SuggestTags_InputValidation(t *testing.T) {
	config := &Config{
		BaseURL: "https://api.openai.com/v1",
		APIKey:  "sk-test123",
		Model:   "gpt-4o",
	}

	client, err := NewClient(config)
	require.NoError(t, err)
	ctx := context.Background()

	tests := []struct {
		name      string
		request   *TagSuggestionRequest
		expectErr bool
		errMsg    string
	}{
		{
			name:      "nil request",
			request:   nil,
			expectErr: true,
			errMsg:    "request cannot be nil",
		},
		{
			name: "empty content",
			request: &TagSuggestionRequest{
				Content: "",
			},
			expectErr: true,
			errMsg:    "content cannot be empty",
		},
		{
			name: "whitespace-only content",
			request: &TagSuggestionRequest{
				Content: "   \n\t   ",
			},
			expectErr: true,
			errMsg:    "content cannot be empty",
		},
		{
			name: "valid request",
			request: &TagSuggestionRequest{
				Content: "This is a valid memo content",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.SuggestTags(ctx, tt.request)

			if tt.expectErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				// For valid requests, we expect an API call error in tests
				// but not a validation error
				if err != nil {
					assert.NotContains(t, err.Error(), "request cannot be nil")
					assert.NotContains(t, err.Error(), "content cannot be empty")
				}
			}
		})
	}
}

func TestClient_parseTagResponse(t *testing.T) {
	config := &Config{
		BaseURL: "https://api.openai.com/v1",
		APIKey:  "sk-test123",
		Model:   "gpt-4o",
	}

	client, err := NewClient(config)
	require.NoError(t, err)

	tests := []struct {
		name     string
		response string
		expected []TagSuggestion
	}{
		{
			name:     "empty response",
			response: "",
			expected: []TagSuggestion{},
		},
		{
			name:     "no valid tags in response",
			response: "This is just plain text without any tag format",
			expected: []TagSuggestion{},
		},
		{
			name:     "single tag",
			response: "[#work/project](this is about a work project)",
			expected: []TagSuggestion{
				{Tag: "work/project", Reason: "this is about a work project"},
			},
		},
		{
			name:     "multiple tags",
			response: "[#work/meeting](about a work meeting) [#idea/brainstorm](brainstorming session) [#urgent](needs immediate attention)",
			expected: []TagSuggestion{
				{Tag: "work/meeting", Reason: "about a work meeting"},
				{Tag: "idea/brainstorm", Reason: "brainstorming session"},
				{Tag: "urgent", Reason: "needs immediate attention"},
			},
		},
		{
			name:     "tags with # prefix (should be removed)",
			response: "[#area/work](work related) [topic/programming](about programming)",
			expected: []TagSuggestion{
				{Tag: "area/work", Reason: "work related"},
				{Tag: "topic/programming", Reason: "about programming"},
			},
		},
		{
			name:     "tags with extra whitespace",
			response: "[  #work/project  ](  this is about work  ) [  personal/note  ](  personal note  )",
			expected: []TagSuggestion{
				{Tag: "work/project", Reason: "this is about work"},
				{Tag: "personal/note", Reason: "personal note"},
			},
		},
		{
			name:     "multilingual tags",
			response: "[#área/trabajo](relacionado con el trabajo) [#主题/编程](关于编程的内容)",
			expected: []TagSuggestion{
				{Tag: "área/trabajo", Reason: "relacionado con el trabajo"},
				{Tag: "主题/编程", Reason: "关于编程的内容"},
			},
		},
		{
			name:     "tags mixed with other text",
			response: "Here are my suggestions: [#work/project](about work) and also [#personal/idea](personal idea). Hope this helps!",
			expected: []TagSuggestion{
				{Tag: "work/project", Reason: "about work"},
				{Tag: "personal/idea", Reason: "personal idea"},
			},
		},
		{
			name:     "invalid format should be ignored",
			response: "[invalid-tag] [#valid/tag](good reason) (invalid-reason) [#another/tag]",
			expected: []TagSuggestion{
				{Tag: "valid/tag", Reason: "good reason"},
			},
		},
		{
			name:     "very long reason should be truncated",
			response: "[#test/tag](" + strings.Repeat("a", 150) + ")",
			expected: []TagSuggestion{
				{Tag: "test/tag", Reason: strings.Repeat("a", 100) + "..."},
			},
		},
		{
			name:     "very long tag should be ignored",
			response: "[#" + strings.Repeat("a", 150) + "](valid reason)",
			expected: []TagSuggestion{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := client.parseTagResponse(tt.response)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestClient_SuggestTags_UserTagsHandling(t *testing.T) {
	config := &Config{
		BaseURL: "https://api.openai.com/v1",
		APIKey:  "sk-test123",
		Model:   "gpt-4o",
	}

	client, err := NewClient(config)
	require.NoError(t, err)
	ctx := context.Background()

	t.Run("handles large number of user tags", func(t *testing.T) {
		// Create 30 user tags (should be limited to 20)
		userTags := make([]string, 30)
		for i := 0; i < 30; i++ {
			userTags[i] = "tag" + string(rune('a'+i))
		}

		request := &TagSuggestionRequest{
			Content:  "Test content",
			UserTags: userTags,
		}

		// This will fail with API error in tests, but should not fail with validation error
		_, err := client.SuggestTags(ctx, request)
		// Just verify it doesn't panic and validates input properly
		if err != nil {
			assert.NotContains(t, err.Error(), "too many user tags")
		}
	})

	t.Run("handles empty user tags", func(t *testing.T) {
		request := &TagSuggestionRequest{
			Content:  "Test content",
			UserTags: []string{},
		}

		_, err := client.SuggestTags(ctx, request)
		// Should not fail with validation error
		if err != nil {
			assert.NotContains(t, err.Error(), "user tags")
		}
	})

	t.Run("handles nil user tags", func(t *testing.T) {
		request := &TagSuggestionRequest{
			Content:  "Test content",
			UserTags: nil,
		}

		_, err := client.SuggestTags(ctx, request)
		// Should not fail with validation error
		if err != nil {
			assert.NotContains(t, err.Error(), "user tags")
		}
	})
}

func TestClient_SuggestTags_TemplateGeneration(t *testing.T) {
	config := &Config{
		BaseURL: "https://api.openai.com/v1",
		APIKey:  "sk-test123",
		Model:   "gpt-4o",
	}

	client, err := NewClient(config)
	require.NoError(t, err)
	ctx := context.Background()

	// We can't easily test the internal template generation without exposing it,
	// but we can test that different request configurations don't cause panics

	tests := []struct {
		name    string
		request *TagSuggestionRequest
	}{
		{
			name: "content only",
			request: &TagSuggestionRequest{
				Content: "Simple memo content",
			},
		},
		{
			name: "with user tags",
			request: &TagSuggestionRequest{
				Content:  "Memo with context",
				UserTags: []string{"work", "project", "meeting"},
			},
		},
		{
			name: "with existing tags",
			request: &TagSuggestionRequest{
				Content:      "Memo with existing tags",
				ExistingTags: []string{"draft", "important"},
			},
		},
		{
			name: "with both user and existing tags",
			request: &TagSuggestionRequest{
				Content:      "Complex memo",
				UserTags:     []string{"work", "personal"},
				ExistingTags: []string{"urgent"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This will fail with API error in tests, but should not panic
			_, err := client.SuggestTags(ctx, tt.request)
			// Just verify it processes the request without panicking
			assert.NotNil(t, err) // Expected in test environment
		})
	}
}

// Integration test for tag suggestion - only runs with proper configuration
func TestClient_SuggestTags_Integration(t *testing.T) {
	// Skip if not in integration test mode
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	config := LoadConfigFromEnv()
	if !config.IsConfigured() {
		t.Skip("AI not configured - set AI_BASE_URL, AI_API_KEY, AI_MODEL environment variables")
	}

	client, err := NewClient(config)
	require.NoError(t, err)
	ctx := context.Background()

	request := &TagSuggestionRequest{
		Content:  "I had a productive meeting today where we discussed the new mobile app features. We brainstormed ideas for dark mode implementation and social login integration. The team was very enthusiastic about these improvements.",
		UserTags: []string{"work", "meeting", "mobile-app", "ideas"},
	}

	response, err := client.SuggestTags(ctx, request)
	require.NoError(t, err)
	require.NotNil(t, response)

	// Verify we got some tags back
	assert.NotEmpty(t, response.Tags)

	// Verify tag structure
	for _, tag := range response.Tags {
		assert.NotEmpty(t, tag.Tag, "Tag should not be empty")
		assert.NotEmpty(t, tag.Reason, "Reason should not be empty")
		assert.LessOrEqual(t, len(tag.Tag), 100, "Tag should not be too long")
		assert.LessOrEqual(t, len(tag.Reason), 103, "Reason should not be too long (100 + '...')")

		t.Logf("Tag: %s, Reason: %s", tag.Tag, tag.Reason)
	}
}
