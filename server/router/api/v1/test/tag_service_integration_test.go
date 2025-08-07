package v1

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	v1pb "github.com/usememos/memos/proto/gen/api/v1"
	"github.com/usememos/memos/server/runner/memopayload"
	"github.com/usememos/memos/store"
)

// TestTagServiceIntegration tests the new TagService and modified MemoService APIs
func TestTagServiceIntegration(t *testing.T) {
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
		Content:    "This is a memo with #work and #project1 tags",
		Visibility: store.Private,
	}
	memo1, err = ts.Store.CreateMemo(ctx, memo1)
	require.NoError(t, err)

	memo2 := &store.Memo{
		UID:        "memo2", 
		CreatorID:  user.ID,
		Content:    "Another memo with #work and #personal tags",
		Visibility: store.Private,
	}
	memo2, err = ts.Store.CreateMemo(ctx, memo2)
	require.NoError(t, err)

	memo3 := &store.Memo{
		UID:        "memo3",
		CreatorID:  user.ID,
		Content:    "Hierarchical tags: #work/project1/backend",
		Visibility: store.Private,
	}
	memo3, err = ts.Store.CreateMemo(ctx, memo3)
	require.NoError(t, err)

	// Rebuild payloads for memos to populate tags
	require.NoError(t, memopayload.RebuildMemoPayload(memo1))
	require.NoError(t, memopayload.RebuildMemoPayload(memo2))
	require.NoError(t, memopayload.RebuildMemoPayload(memo3))
	
	// Update memos with rebuilt payloads
	err = ts.Store.UpdateMemo(ctx, &store.UpdateMemo{ID: memo1.ID, Payload: memo1.Payload})
	require.NoError(t, err)
	err = ts.Store.UpdateMemo(ctx, &store.UpdateMemo{ID: memo2.ID, Payload: memo2.Payload})
	require.NoError(t, err)
	err = ts.Store.UpdateMemo(ctx, &store.UpdateMemo{ID: memo3.ID, Payload: memo3.Payload})
	require.NoError(t, err)

	t.Run("TagService_ListTags", func(t *testing.T) {
		resp, err := ts.Service.ListTags(authCtx, &v1pb.ListTagsRequest{
			IncludeMemoIds: true,
		})
		require.NoError(t, err)
		require.NotNil(t, resp)

		// Should have at least 4 tags: work, project1, personal, work/project1/backend
		assert.GreaterOrEqual(t, len(resp.Tags), 3)
		assert.Equal(t, int32(len(resp.Tags)), resp.TotalCount)

		// Check that tags have memo associations
		found_work_tag := false
		for _, tag := range resp.Tags {
			if tag.TagNode.Name == "work" || tag.TagNode.Name == "/work" {
				found_work_tag = true
				assert.GreaterOrEqual(t, len(tag.TagNode.MemoIds), 1)
				assert.Greater(t, tag.DirectMemoCount, int32(0))
			}
		}
		assert.True(t, found_work_tag, "Should find work tag")
	})

	t.Run("TagService_GetTag", func(t *testing.T) {
		resp, err := ts.Service.GetTag(authCtx, &v1pb.GetTagRequest{
			TagPath:         "/work",  // Use the correct tag path format
			IncludeMemoIds: true,
		})
		require.NoError(t, err)
		require.NotNil(t, resp.Tag)

		assert.Contains(t, []string{"work", "/work"}, resp.Tag.TagNode.Name)
		assert.GreaterOrEqual(t, len(resp.Tag.TagNode.MemoIds), 1)
	})

	t.Run("TagService_GetTag_NotFound", func(t *testing.T) {
		_, err := ts.Service.GetTag(authCtx, &v1pb.GetTagRequest{
			TagPath: "nonexistent",
		})
		assert.Error(t, err)
		assert.Equal(t, codes.NotFound, status.Code(err))
	})

	t.Run("MemoService_RenameMemoTag_SingleMemo_Success", func(t *testing.T) {
		// This should still work - renaming tag in a single memo
		_, err := ts.Service.RenameMemoTag(authCtx, &v1pb.RenameMemoTagRequest{
			Parent: "memos/" + memo1.UID,
			OldTag: "project1", 
			NewTag: "project2",
		})
		require.NoError(t, err)

		// Verify memo content was updated
		updatedMemo, err := ts.Store.GetMemo(ctx, &store.FindMemo{UID: &memo1.UID})
		require.NoError(t, err)
		assert.Contains(t, updatedMemo.Content, "#project2")
		assert.NotContains(t, updatedMemo.Content, "#project1")
	})

	t.Run("MemoService_RenameMemoTag_GlobalOperation_Rejected", func(t *testing.T) {
		// This should be rejected - global operations no longer supported
		_, err := ts.Service.RenameMemoTag(authCtx, &v1pb.RenameMemoTagRequest{
			Parent: "memos/-",  // Global operation
			OldTag: "work",
			NewTag: "office", 
		})
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
		assert.Contains(t, err.Error(), "Global tag operations are no longer supported")
	})

	t.Run("MemoService_DeleteMemoTag_SingleMemo_Success", func(t *testing.T) {
		// This should still work - deleting tag from a single memo
		_, err := ts.Service.DeleteMemoTag(authCtx, &v1pb.DeleteMemoTagRequest{
			Parent: "memos/" + memo2.UID,
			Tag:    "personal",
		})
		require.NoError(t, err)

		// Verify tag was removed from memo content
		updatedMemo, err := ts.Store.GetMemo(ctx, &store.FindMemo{UID: &memo2.UID})
		require.NoError(t, err)
		assert.NotContains(t, updatedMemo.Content, "#personal")
	})

	t.Run("MemoService_DeleteMemoTag_GlobalOperation_Rejected", func(t *testing.T) {
		// This should be rejected - global operations no longer supported
		_, err := ts.Service.DeleteMemoTag(authCtx, &v1pb.DeleteMemoTagRequest{
			Parent: "memos/-",  // Global operation
			Tag:    "work",
		})
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
		assert.Contains(t, err.Error(), "Global tag operations are no longer supported")
	})

	t.Run("MemoService_BatchDeleteMemosByTag_DryRun", func(t *testing.T) {
		// Test dry run first
		resp, err := ts.Service.BatchDeleteMemosByTag(authCtx, &v1pb.BatchDeleteMemosByTagRequest{
			TagPath: "work",
			DryRun:  true,
		})
		require.NoError(t, err)
		
		// Should find memos that would be deleted
		assert.Greater(t, resp.DeletedCount, int32(0))
		assert.Greater(t, len(resp.DeletedMemoIds), 0)
		assert.Contains(t, resp.AffectedTagPaths, "work")
		
		// Verify memos still exist (dry run)
		memo, err := ts.Store.GetMemo(ctx, &store.FindMemo{UID: &memo1.UID})
		require.NoError(t, err)
		assert.NotNil(t, memo)
	})

	t.Run("TagService_RenameTag_Global", func(t *testing.T) {
		// Create a fresh memo for this test
		testMemo := &store.Memo{
			UID:        "test-rename",
			CreatorID:  user.ID,
			Content:    "Testing rename with #testing tag",
			Visibility: store.Private,
		}
		testMemo, err := ts.Store.CreateMemo(ctx, testMemo)
		require.NoError(t, err)
		require.NoError(t, memopayload.RebuildMemoPayload(testMemo))
		err = ts.Store.UpdateMemo(ctx, &store.UpdateMemo{ID: testMemo.ID, Payload: testMemo.Payload})
		require.NoError(t, err)
		
		// Test global rename
		resp, err := ts.Service.RenameTag(authCtx, &v1pb.RenameTagRequest{
			OldTagPath: "testing",
			NewTagPath: "/renamed-testing",
		})
		require.NoError(t, err)
		assert.Contains(t, resp.AffectedMemoIds, testMemo.UID)
		
		// Verify memo content was updated
		updatedMemo, err := ts.Store.GetMemo(ctx, &store.FindMemo{UID: &testMemo.UID})
		require.NoError(t, err)
		assert.Contains(t, updatedMemo.Content, "#renamed-testing")
		assert.NotContains(t, updatedMemo.Content, "#testing")
	})

	t.Run("TagService_DeleteTag_RemoveFromContent", func(t *testing.T) {
		// Create a fresh memo for this test
		testMemo := &store.Memo{
			UID:        "test-delete",
			CreatorID:  user.ID,
			Content:    "Testing delete with #deleteme tag",
			Visibility: store.Private,
		}
		testMemo, err := ts.Store.CreateMemo(ctx, testMemo)
		require.NoError(t, err)
		require.NoError(t, memopayload.RebuildMemoPayload(testMemo))
		err = ts.Store.UpdateMemo(ctx, &store.UpdateMemo{ID: testMemo.ID, Payload: testMemo.Payload})
		require.NoError(t, err)
		
		// Test tag deletion (remove from content)
		resp, err := ts.Service.DeleteTag(authCtx, &v1pb.DeleteTagRequest{
			TagPath:  "deleteme",
			Strategy: v1pb.DeleteTagRequest_REMOVE_FROM_CONTENT,
		})
		require.NoError(t, err)
		assert.Contains(t, resp.AffectedMemoIds, testMemo.UID)
		
		// Verify tag was removed from content but memo still exists
		updatedMemo, err := ts.Store.GetMemo(ctx, &store.FindMemo{UID: &testMemo.UID})
		require.NoError(t, err)
		assert.NotContains(t, updatedMemo.Content, "#deleteme")
	})
}

