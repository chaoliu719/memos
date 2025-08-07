package v1

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	v1pb "github.com/usememos/memos/proto/gen/api/v1"
)

// TestMemoTagGlobalOperationsRejection tests that global tag operations are properly rejected
func TestMemoTagGlobalOperationsRejection(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	// Create a test user
	user, err := ts.CreateRegularUser(ctx, "testuser")
	require.NoError(t, err)

	// Create authenticated context
	authCtx := ts.CreateUserContext(ctx, user.ID)

	t.Run("RenameMemoTag_GlobalOperation_Rejected", func(t *testing.T) {
		// This should be rejected - global operations no longer supported
		_, err := ts.Service.RenameMemoTag(authCtx, &v1pb.RenameMemoTagRequest{
			Parent: "memos/-",  // Global operation
			OldTag: "work",
			NewTag: "office", 
		})
		
		// Verify error
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
		assert.Contains(t, err.Error(), "Global tag operations are no longer supported")
		assert.Contains(t, err.Error(), "TagService.RenameTag")
	})

	t.Run("DeleteMemoTag_GlobalOperation_Rejected", func(t *testing.T) {
		// This should be rejected - global operations no longer supported  
		_, err := ts.Service.DeleteMemoTag(authCtx, &v1pb.DeleteMemoTagRequest{
			Parent: "memos/-",  // Global operation
			Tag:    "work",
		})
		
		// Verify error
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
		assert.Contains(t, err.Error(), "Global tag operations are no longer supported")
		assert.Contains(t, err.Error(), "TagService.DeleteTag")
	})
}