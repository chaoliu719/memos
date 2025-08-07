package v1

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1pb "github.com/usememos/memos/proto/gen/api/v1"
	"github.com/usememos/memos/server/runner/memopayload"
	"github.com/usememos/memos/store"
)

// TestMemoTagSingleOperations tests that single memo tag operations still work
func TestMemoTagSingleOperations(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	// Create a test user
	user, err := ts.CreateRegularUser(ctx, "testuser")
	require.NoError(t, err)

	// Create authenticated context
	authCtx := ts.CreateUserContext(ctx, user.ID)

	t.Run("RenameMemoTag_SingleMemo_Success", func(t *testing.T) {
		// Create a test memo with a tag
		memo := &store.Memo{
			UID:        "test-memo-rename",
			CreatorID:  user.ID,
			Content:    "This memo has #oldtag",
			Visibility: store.Private,
		}
		memo, err := ts.Store.CreateMemo(ctx, memo)
		require.NoError(t, err)
		
		// Rebuild payload to extract tags
		require.NoError(t, memopayload.RebuildMemoPayload(memo))
		err = ts.Store.UpdateMemo(ctx, &store.UpdateMemo{ID: memo.ID, Payload: memo.Payload})
		require.NoError(t, err)

		// Test renaming tag in single memo - should work
		_, err = ts.Service.RenameMemoTag(authCtx, &v1pb.RenameMemoTagRequest{
			Parent: "memos/" + memo.UID,  // Single memo operation
			OldTag: "oldtag",
			NewTag: "newtag",
		})
		require.NoError(t, err)

		// Verify memo content was updated
		updatedMemo, err := ts.Store.GetMemo(ctx, &store.FindMemo{UID: &memo.UID})
		require.NoError(t, err)
		assert.Contains(t, updatedMemo.Content, "#newtag")
		assert.NotContains(t, updatedMemo.Content, "#oldtag")
	})

	t.Run("DeleteMemoTag_SingleMemo_Success", func(t *testing.T) {
		// Create a test memo with a tag
		memo := &store.Memo{
			UID:        "test-memo-delete",
			CreatorID:  user.ID,
			Content:    "This memo has #removeme and other content",
			Visibility: store.Private,
		}
		memo, err := ts.Store.CreateMemo(ctx, memo)
		require.NoError(t, err)
		
		// Rebuild payload to extract tags
		require.NoError(t, memopayload.RebuildMemoPayload(memo))
		err = ts.Store.UpdateMemo(ctx, &store.UpdateMemo{ID: memo.ID, Payload: memo.Payload})
		require.NoError(t, err)

		// Test deleting tag from single memo - should work
		_, err = ts.Service.DeleteMemoTag(authCtx, &v1pb.DeleteMemoTagRequest{
			Parent: "memos/" + memo.UID,  // Single memo operation
			Tag:    "removeme",
		})
		require.NoError(t, err)

		// Verify tag was removed from memo content
		updatedMemo, err := ts.Store.GetMemo(ctx, &store.FindMemo{UID: &memo.UID})
		require.NoError(t, err)
		assert.NotContains(t, updatedMemo.Content, "#removeme")
		assert.Contains(t, updatedMemo.Content, "other content") // Other content should remain
	})
}