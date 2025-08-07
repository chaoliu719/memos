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

// TestTagServiceBasicFunctionality tests basic TagService functionality
func TestTagServiceBasicFunctionality(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	// Create a test user
	user, err := ts.CreateRegularUser(ctx, "testuser")
	require.NoError(t, err)

	// Create authenticated context
	authCtx := ts.CreateUserContext(ctx, user.ID)

	// Create test memos with tags
	memo1 := &store.Memo{
		UID:        "memo1",
		CreatorID:  user.ID,
		Content:    "This memo has #work tag",
		Visibility: store.Private,
	}
	memo1, err = ts.Store.CreateMemo(ctx, memo1)
	require.NoError(t, err)

	memo2 := &store.Memo{
		UID:        "memo2",
		CreatorID:  user.ID,
		Content:    "This memo has #work and #project tags",
		Visibility: store.Private,
	}
	memo2, err = ts.Store.CreateMemo(ctx, memo2)
	require.NoError(t, err)

	// Rebuild payloads to extract tags
	require.NoError(t, memopayload.RebuildMemoPayload(memo1))
	require.NoError(t, memopayload.RebuildMemoPayload(memo2))
	
	err = ts.Store.UpdateMemo(ctx, &store.UpdateMemo{ID: memo1.ID, Payload: memo1.Payload})
	require.NoError(t, err)
	err = ts.Store.UpdateMemo(ctx, &store.UpdateMemo{ID: memo2.ID, Payload: memo2.Payload})
	require.NoError(t, err)

	t.Run("TagService_ListTags", func(t *testing.T) {
		resp, err := ts.Service.ListTags(authCtx, &v1pb.ListTagsRequest{
			IncludeMemoIds: true,
		})
		require.NoError(t, err)
		require.NotNil(t, resp)

		// Should have tags
		assert.Greater(t, len(resp.Tags), 0)
		assert.Equal(t, int32(len(resp.Tags)), resp.TotalCount)

		// Find work tag and verify it has memo associations
		workTagFound := false
		for _, tag := range resp.Tags {
			// Check for both "work" and "/work" formats
			if tag.TagNode.Name == "work" || tag.TagNode.Name == "/work" {
				workTagFound = true
				assert.GreaterOrEqual(t, len(tag.TagNode.MemoIds), 1, "work tag should have memo associations")
				assert.Greater(t, tag.DirectMemoCount, int32(0), "work tag should have direct memo count > 0")
				break
			}
		}
		assert.True(t, workTagFound, "Should find work tag")
	})

	t.Run("BatchDeleteMemosByTag_DryRun", func(t *testing.T) {
		// Test dry run
		resp, err := ts.Service.BatchDeleteMemosByTag(authCtx, &v1pb.BatchDeleteMemosByTagRequest{
			TagPath: "work",
			DryRun:  true,
		})
		require.NoError(t, err)
		
		// Should find memos that would be deleted
		assert.Greater(t, resp.DeletedCount, int32(0))
		assert.Greater(t, len(resp.DeletedMemoIds), 0)
		
		// Verify memos still exist (dry run)
		memo, err := ts.Store.GetMemo(ctx, &store.FindMemo{UID: &memo1.UID})
		require.NoError(t, err)
		assert.NotNil(t, memo)
	})

	t.Run("TagService_RenameTag_Global", func(t *testing.T) {
		// Create a fresh memo for this test to avoid conflicts
		testMemo := &store.Memo{
			UID:        "test-global-rename",
			CreatorID:  user.ID,
			Content:    "Testing global rename with #globaltest tag",
			Visibility: store.Private,
		}
		testMemo, err := ts.Store.CreateMemo(ctx, testMemo)
		require.NoError(t, err)
		require.NoError(t, memopayload.RebuildMemoPayload(testMemo))
		err = ts.Store.UpdateMemo(ctx, &store.UpdateMemo{ID: testMemo.ID, Payload: testMemo.Payload})
		require.NoError(t, err)
		
		// Test global rename
		resp, err := ts.Service.RenameTag(authCtx, &v1pb.RenameTagRequest{
			OldTagPath: "globaltest",
			NewTagPath: "renamed-global",
		})
		require.NoError(t, err)
		assert.Contains(t, resp.AffectedMemoIds, testMemo.UID)
		
		// Verify memo content was updated
		updatedMemo, err := ts.Store.GetMemo(ctx, &store.FindMemo{UID: &testMemo.UID})
		require.NoError(t, err)
		assert.Contains(t, updatedMemo.Content, "#renamed-global")
		assert.NotContains(t, updatedMemo.Content, "#globaltest")
	})
}