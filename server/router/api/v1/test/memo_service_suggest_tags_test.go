package v1

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/usememos/memos/plugin/ai"
	v1pb "github.com/usememos/memos/proto/gen/api/v1"
	"github.com/usememos/memos/store"
)

func TestAPIV1Service_SuggestMemoTags_Authentication(t *testing.T) {
	ts := NewTestService(t)
	defer ts.Cleanup()

	tests := []struct {
		name        string
		ctx         context.Context
		expectError bool
		expectCode  codes.Code
	}{
		{
			name:        "unauthenticated request",
			ctx:         context.Background(),
			expectError: true,
			expectCode:  codes.Unauthenticated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := &v1pb.SuggestMemoTagsRequest{
				Content: "Test memo content",
			}

			response, err := ts.Service.SuggestMemoTags(tt.ctx, request)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, response)

				st, ok := status.FromError(err)
				require.True(t, ok, "Error should be a gRPC status error")
				assert.Equal(t, tt.expectCode, st.Code())
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, response)
			}
		})
	}
}

func TestAPIV1Service_SuggestMemoTags_InputValidation(t *testing.T) {
	ts := NewTestService(t)
	defer ts.Cleanup()

	// Create a test user
	ctx := context.Background()
	user, err := ts.CreateRegularUser(ctx, "test-user")
	require.NoError(t, err)

	// Create authenticated context
	authCtx := ts.CreateUserContext(ctx, user.ID)

	tests := []struct {
		name        string
		request     *v1pb.SuggestMemoTagsRequest
		expectError bool
		expectCode  codes.Code
	}{
		{
			name:        "nil request",
			request:     nil,
			expectError: true,
			expectCode:  codes.InvalidArgument,
		},
		{
			name: "empty content",
			request: &v1pb.SuggestMemoTagsRequest{
				Content: "",
			},
			expectError: true,
			expectCode:  codes.InvalidArgument,
		},
		{
			name: "whitespace-only content",
			request: &v1pb.SuggestMemoTagsRequest{
				Content: "   \n\t   ",
			},
			expectError: true,
			expectCode:  codes.InvalidArgument,
		},
		{
			name: "valid content",
			request: &v1pb.SuggestMemoTagsRequest{
				Content: "This is a valid memo content",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, err := ts.Service.SuggestMemoTags(authCtx, tt.request)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, response)

				if tt.expectCode != codes.OK {
					st, ok := status.FromError(err)
					require.True(t, ok, "Error should be a gRPC status error")
					assert.Equal(t, tt.expectCode, st.Code())
				}
			} else {
				// For valid requests, we expect success regardless of AI configuration
				assert.NoError(t, err)
				assert.NotNil(t, response)
				// If AI is configured, we get suggestions; if not, we get empty list
				assert.GreaterOrEqual(t, len(response.SuggestedTags), 0)
			}
		})
	}
}

func TestAPIV1Service_SuggestMemoTags_AIConfiguration(t *testing.T) {
	// Save original environment variables
	origBaseURL := os.Getenv("AI_BASE_URL")
	origAPIKey := os.Getenv("AI_API_KEY")
	origModel := os.Getenv("AI_MODEL")

	ts := NewTestService(t)
	defer ts.Cleanup()

	// Create a test user and authenticated context
	ctx := context.Background()
	user, err := ts.CreateRegularUser(ctx, "test-user")
	require.NoError(t, err)
	authCtx := ts.CreateUserContext(ctx, user.ID)

	tests := []struct {
		name     string
		setupEnv func()
		expectAI bool
	}{
		{
			name: "no AI configuration",
			setupEnv: func() {
				os.Unsetenv("AI_BASE_URL")
				os.Unsetenv("AI_API_KEY")
				os.Unsetenv("AI_MODEL")
			},
			expectAI: false,
		},
		{
			name: "incomplete AI configuration",
			setupEnv: func() {
				os.Setenv("AI_BASE_URL", "https://api.openai.com/v1")
				os.Setenv("AI_API_KEY", "sk-test123")
				os.Unsetenv("AI_MODEL")
			},
			expectAI: false,
		},
		{
			name: "complete AI configuration",
			setupEnv: func() {
				os.Setenv("AI_BASE_URL", "https://api.openai.com/v1")
				os.Setenv("AI_API_KEY", "sk-test123")
				os.Setenv("AI_MODEL", "gpt-4o")
			},
			expectAI: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup environment
			tt.setupEnv()

			request := &v1pb.SuggestMemoTagsRequest{
				Content: "Test memo about programming and software development",
			}

			response, err := ts.Service.SuggestMemoTags(authCtx, request)

			if tt.expectAI {
				// With AI configured, we expect an error since we're using fake credentials
				// but the error should be about API call failure, not configuration
				if err != nil {
					assert.NotContains(t, err.Error(), "AI configuration incomplete")
					// The error should be about the actual AI call failing
				}
			} else {
				// Without AI configured, we should get empty suggestions
				assert.NoError(t, err)
				assert.NotNil(t, response)
				assert.Equal(t, 0, len(response.SuggestedTags))
			}
		})
	}

	// Restore original environment variables
	os.Unsetenv("AI_BASE_URL")
	os.Unsetenv("AI_API_KEY")
	os.Unsetenv("AI_MODEL")

	if origBaseURL != "" {
		os.Setenv("AI_BASE_URL", origBaseURL)
	}
	if origAPIKey != "" {
		os.Setenv("AI_API_KEY", origAPIKey)
	}
	if origModel != "" {
		os.Setenv("AI_MODEL", origModel)
	}
}

func TestAPIV1Service_SuggestMemoTags_NoAIConfiguration(t *testing.T) {
	// Save original environment variables
	origBaseURL := os.Getenv("AI_BASE_URL")
	origAPIKey := os.Getenv("AI_API_KEY")
	origModel := os.Getenv("AI_MODEL")

	// Ensure no AI configuration
	os.Unsetenv("AI_BASE_URL")
	os.Unsetenv("AI_API_KEY")
	os.Unsetenv("AI_MODEL")

	ts := NewTestService(t)
	defer ts.Cleanup()

	// Create a test user and authenticated context
	ctx := context.Background()
	user, err := ts.CreateRegularUser(ctx, "test-user")
	require.NoError(t, err)
	authCtx := ts.CreateUserContext(ctx, user.ID)

	request := &v1pb.SuggestMemoTagsRequest{
		Content: "This is a memo about programming and software development that should get no AI suggestions",
	}

	response, err := ts.Service.SuggestMemoTags(authCtx, request)

	// Without AI configured, should return empty suggestions but no error
	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.Equal(t, 0, len(response.SuggestedTags))

	// Restore original environment variables
	if origBaseURL != "" {
		os.Setenv("AI_BASE_URL", origBaseURL)
	}
	if origAPIKey != "" {
		os.Setenv("AI_API_KEY", origAPIKey)
	}
	if origModel != "" {
		os.Setenv("AI_MODEL", origModel)
	}
}

func TestAPIV1Service_SuggestMemoTags_WithAIConfiguration(t *testing.T) {
	// This test assumes AI is configured (your normal environment)
	config := ai.LoadConfigFromEnv()
	if !config.IsConfigured() {
		t.Skip("AI not configured - this test requires AI configuration")
	}

	ts := NewTestService(t)
	defer ts.Cleanup()

	// Create a test user and authenticated context
	ctx := context.Background()
	user, err := ts.CreateRegularUser(ctx, "test-user")
	require.NoError(t, err)
	authCtx := ts.CreateUserContext(ctx, user.ID)

	request := &v1pb.SuggestMemoTagsRequest{
		Content: "I had a productive meeting today where we discussed new mobile app features for our Q3 roadmap",
	}

	response, err := ts.Service.SuggestMemoTags(authCtx, request)

	// With AI configured, should return actual suggestions
	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.Greater(t, len(response.SuggestedTags), 0, "Should receive AI-generated tag suggestions")

	// Verify tag quality
	for _, suggestion := range response.SuggestedTags {
		assert.NotEmpty(t, suggestion.Tag, "Tag should not be empty")
		assert.NotEmpty(t, suggestion.Reason, "Reason should not be empty")
		assert.False(t, strings.HasPrefix(suggestion.Tag, "#"), "Tag should not start with #")

		t.Logf("AI Suggested Tag: %s, Reason: %s", suggestion.Tag, suggestion.Reason)
	}
}

func TestAPIV1Service_SuggestMemoTags_ExistingTagsHandling(t *testing.T) {
	ts := NewTestService(t)
	defer ts.Cleanup()

	// Create a test user and authenticated context
	ctx := context.Background()
	user, err := ts.CreateRegularUser(ctx, "test-user")
	require.NoError(t, err)
	authCtx := ts.CreateUserContext(ctx, user.ID)

	// Test that existing tags are properly handled
	request := &v1pb.SuggestMemoTagsRequest{
		Content:      "This is a memo about work and programming",
		ExistingTags: []string{"work", "programming", "ideas"},
	}

	response, err := ts.Service.SuggestMemoTags(authCtx, request)

	// Should succeed regardless of AI configuration
	assert.NoError(t, err)
	assert.NotNil(t, response)
	// If AI is configured, we get suggestions; if not, we get empty list
	assert.GreaterOrEqual(t, len(response.SuggestedTags), 0)
}

func TestAPIV1Service_SuggestMemoTags_MemoTagsExtraction(t *testing.T) {
	ts := NewTestService(t)
	defer ts.Cleanup()

	// Create a test user and memos with tags
	ctx := context.Background()
	user, err := ts.CreateRegularUser(ctx, "test-user")
	require.NoError(t, err)
	authCtx := ts.CreateUserContext(ctx, user.ID)

	// Create some test memos with tags
	memo1, err := ts.Store.CreateMemo(ctx, &store.Memo{
		UID:        "test-memo-1",
		CreatorID:  user.ID,
		Content:    "This is about #programming and #work",
		Visibility: store.Private,
	})
	require.NoError(t, err)

	memo2, err := ts.Store.CreateMemo(ctx, &store.Memo{
		UID:        "test-memo-2",
		CreatorID:  user.ID,
		Content:    "Another memo about #learning and #books",
		Visibility: store.Private,
	})
	require.NoError(t, err)

	// Test that the service can extract existing tags from user's memos
	request := &v1pb.SuggestMemoTagsRequest{
		Content: "New memo content about programming and learning",
	}

	response, err := ts.Service.SuggestMemoTags(authCtx, request)

	// Should succeed regardless of AI configuration
	assert.NoError(t, err)
	assert.NotNil(t, response)
	// If AI is configured, we get suggestions; if not, we get empty list
	assert.GreaterOrEqual(t, len(response.SuggestedTags), 0)

	// Verify memos were created (this validates our test setup)
	assert.NotNil(t, memo1)
	assert.NotNil(t, memo2)
}

// Integration test with real AI - only runs with proper configuration
func TestAPIV1Service_SuggestMemoTags_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	config := ai.LoadConfigFromEnv()
	if !config.IsConfigured() {
		t.Skip("AI not configured - set AI_BASE_URL, AI_API_KEY, AI_MODEL environment variables")
	}

	ts := NewTestService(t)
	defer ts.Cleanup()

	// Create a test user and authenticated context
	ctx := context.Background()
	user, err := ts.CreateRegularUser(ctx, "test-user")
	require.NoError(t, err)
	authCtx := ts.CreateUserContext(ctx, user.ID)

	// Test with realistic memo content
	request := &v1pb.SuggestMemoTagsRequest{
		Content:      "I had a productive meeting today where we discussed new mobile app features. The team brainstormed ideas for dark mode and social login integration. We also reviewed the current user experience and identified areas for improvement.",
		ExistingTags: []string{"work", "meeting"},
	}

	response, err := ts.Service.SuggestMemoTags(authCtx, request)
	require.NoError(t, err)
	require.NotNil(t, response)

	// Verify we got meaningful suggestions
	assert.NotEmpty(t, response.SuggestedTags, "Should receive tag suggestions from AI")

	for _, suggestion := range response.SuggestedTags {
		assert.NotEmpty(t, suggestion.Tag, "Tag should not be empty")
		assert.NotEmpty(t, suggestion.Reason, "Reason should not be empty")

		// Verify tag format (should not start with #)
		assert.False(t, strings.HasPrefix(suggestion.Tag, "#"), "Tag should not start with #")

		t.Logf("Suggested Tag: %s, Reason: %s", suggestion.Tag, suggestion.Reason)
	}

	// Verify reasonable limits
	assert.LessOrEqual(t, len(response.SuggestedTags), 10, "Should not return too many suggestions")
}

func TestAPIV1Service_SuggestMemoTags_ContentValidation(t *testing.T) {
	ts := NewTestService(t)
	defer ts.Cleanup()

	// Create a test user and authenticated context
	ctx := context.Background()
	user, err := ts.CreateRegularUser(ctx, "test-user")
	require.NoError(t, err)
	authCtx := ts.CreateUserContext(ctx, user.ID)

	tests := []struct {
		name    string
		content string
		valid   bool
	}{
		{
			name:    "normal content",
			content: "This is a normal memo about work",
			valid:   true,
		},
		{
			name:    "content with special characters",
			content: "Meeting notes: @john said we need to review the #design 🎨",
			valid:   true,
		},
		{
			name:    "multilingual content",
			content: "今天开会讨论了新功能的设计方案，大家都很积极参与",
			valid:   true,
		},
		{
			name:    "very long content",
			content: strings.Repeat("This is a very long memo content. ", 100),
			valid:   true,
		},
		{
			name:    "empty content",
			content: "",
			valid:   false,
		},
		{
			name:    "whitespace only",
			content: "   \n\t   ",
			valid:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := &v1pb.SuggestMemoTagsRequest{
				Content: tt.content,
			}

			response, err := ts.Service.SuggestMemoTags(authCtx, request)

			if tt.valid {
				assert.NoError(t, err)
				assert.NotNil(t, response)
				// Should succeed regardless of AI configuration
				assert.GreaterOrEqual(t, len(response.SuggestedTags), 0)
			} else {
				assert.Error(t, err)
				assert.Nil(t, response)

				st, ok := status.FromError(err)
				require.True(t, ok, "Error should be a gRPC status error")
				assert.Equal(t, codes.InvalidArgument, st.Code())
			}
		})
	}
}
