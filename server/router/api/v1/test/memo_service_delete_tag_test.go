package v1

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	v1pb "github.com/usememos/memos/proto/gen/api/v1"
	"github.com/usememos/memos/server/runner/memopayload"
	"github.com/usememos/memos/store"
)

func TestDeleteMemoTag_RemoveTagOnly(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	// Create a test user
	user, err := ts.CreateHostUser(ctx, "test_user")
	require.NoError(t, err)
	userCtx := ts.CreateUserContext(ctx, user.ID)

	// Create a memo with multiple tags
	memo := &store.Memo{
		UID:        "test-memo-1",
		CreatorID:  user.ID,
		Content:    "This is a memo with #work #project #important tags",
		Visibility: store.Public,
	}
	
	// Build payload properly to extract tags from content
	err = memopayload.RebuildMemoPayload(memo)
	require.NoError(t, err)
	
	// Create memo in store
	memo, err = ts.Store.CreateMemo(ctx, memo)
	require.NoError(t, err)
	require.NotNil(t, memo)

	// Delete the "work" tag with delete_related_memos = false (remove tag only)
	memoName := fmt.Sprintf("memos/%s", memo.UID)
	_, err = ts.Service.DeleteMemoTag(userCtx, &v1pb.DeleteMemoTagRequest{
		Parent:              memoName,
		Tag:                 "work",
		DeleteRelatedMemos:  false, // Remove tag only
	})
	require.NoError(t, err)

	// Verify the memo still exists
	updatedMemo, err := ts.Store.GetMemo(ctx, &store.FindMemo{UID: &memo.UID})
	require.NoError(t, err)
	require.NotNil(t, updatedMemo)

	// Verify the content has been updated (tag removed)
	require.Equal(t, "This is a memo with #project #important tags", updatedMemo.Content)

	// Verify the payload tags have been updated
	require.Contains(t, updatedMemo.Payload.Tags, "project")
	require.Contains(t, updatedMemo.Payload.Tags, "important")
	require.NotContains(t, updatedMemo.Payload.Tags, "work")
}

func TestDeleteMemoTag_DeleteMemo(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	// Create a test user
	user, err := ts.CreateHostUser(ctx, "test_user")
	require.NoError(t, err)
	userCtx := ts.CreateUserContext(ctx, user.ID)

	// Create a memo with a tag
	memo := &store.Memo{
		UID:        "test-memo-2",
		CreatorID:  user.ID,
		Content:    "This is a memo with #delete tag",
		Visibility: store.Public,
	}
	
	// Build payload properly to extract tags from content
	err = memopayload.RebuildMemoPayload(memo)
	require.NoError(t, err)
	
	// Create memo in store
	memo, err = ts.Store.CreateMemo(ctx, memo)
	require.NoError(t, err)
	require.NotNil(t, memo)

	// Delete the "delete" tag with delete_related_memos = true (delete memo)
	memoName := fmt.Sprintf("memos/%s", memo.UID)
	_, err = ts.Service.DeleteMemoTag(userCtx, &v1pb.DeleteMemoTagRequest{
		Parent:              memoName,
		Tag:                 "delete",
		DeleteRelatedMemos:  true, // Delete entire memo
	})
	require.NoError(t, err)

	// Verify the memo no longer exists
	deletedMemo, err := ts.Store.GetMemo(ctx, &store.FindMemo{UID: &memo.UID})
	require.NoError(t, err)
	require.Nil(t, deletedMemo)
}

func TestDeleteMemoTag_SmartWhitespaceHandling(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	// Create a test user
	user, err := ts.CreateHostUser(ctx, "test_user")
	require.NoError(t, err)
	userCtx := ts.CreateUserContext(ctx, user.ID)

	testCases := []struct {
		name        string
		content     string
		tagToRemove string
		expected    string
	}{
		{
			name:        "Remove tag between words",
			content:     "Meeting notes #work are important",
			tagToRemove: "work",
			expected:    "Meeting notes are important",
		},
		{
			name:        "Remove tag from multiple tags",
			content:     "Notes #work #project #important",
			tagToRemove: "work",
			expected:    "Notes #project #important",
		},
		{
			name:        "Remove only tag",
			content:     "#solo",
			tagToRemove: "solo",
			expected:    "",
		},
		{
			name:        "Tag not found",
			content:     "This has #other tag",
			tagToRemove: "missing",
			expected:    "This has #other tag",
		},
	}

	for i, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create memo for this test case
			memoUID := fmt.Sprintf("test-memo-whitespace-%d", i)
			memo := &store.Memo{
				UID:        memoUID,
				CreatorID:  user.ID,
				Content:    tc.content,
				Visibility: store.Public,
			}
			
			// Build payload properly to extract tags from content
			err := memopayload.RebuildMemoPayload(memo)
			require.NoError(t, err)
			
			// Create memo in store
			memo, err = ts.Store.CreateMemo(ctx, memo)
			require.NoError(t, err)

			// Remove tag
			memoName := fmt.Sprintf("memos/%s", memo.UID)
			_, err = ts.Service.DeleteMemoTag(userCtx, &v1pb.DeleteMemoTagRequest{
				Parent:              memoName,
				Tag:                 tc.tagToRemove,
				DeleteRelatedMemos:  false, // Remove tag only
			})
			require.NoError(t, err)

			// Verify content
			updatedMemo, err := ts.Store.GetMemo(ctx, &store.FindMemo{UID: &memo.UID})
			require.NoError(t, err)
			require.NotNil(t, updatedMemo)
			require.Equal(t, tc.expected, updatedMemo.Content)
		})
	}
}

func TestDeleteMemoTag_MultipleMemosWithSameTag(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	// Create a test user
	user, err := ts.CreateHostUser(ctx, "test_user")
	require.NoError(t, err)
	userCtx := ts.CreateUserContext(ctx, user.ID)

	// Create multiple memos with the same tag
	memo1 := &store.Memo{
		UID:        "test-memo-multi-1",
		CreatorID:  user.ID,
		Content:    "First memo with #shared tag",
		Visibility: store.Public,
	}
	err = memopayload.RebuildMemoPayload(memo1)
	require.NoError(t, err)
	memo1, err = ts.Store.CreateMemo(ctx, memo1)
	require.NoError(t, err)

	memo2 := &store.Memo{
		UID:        "test-memo-multi-2",
		CreatorID:  user.ID,
		Content:    "Second memo also has #shared tag",
		Visibility: store.Public,
	}
	err = memopayload.RebuildMemoPayload(memo2)
	require.NoError(t, err)
	memo2, err = ts.Store.CreateMemo(ctx, memo2)
	require.NoError(t, err)

	// Delete the "shared" tag from all memos (using memos/- parent)
	_, err = ts.Service.DeleteMemoTag(userCtx, &v1pb.DeleteMemoTagRequest{
		Parent:              "memos/-", // Apply to all memos
		Tag:                 "shared",
		DeleteRelatedMemos:  false, // Remove tag only
	})
	require.NoError(t, err)

	// Verify both memos have been updated
	updatedMemo1, err := ts.Store.GetMemo(ctx, &store.FindMemo{UID: &memo1.UID})
	require.NoError(t, err)
	require.Equal(t, "First memo with tag", updatedMemo1.Content)
	require.NotContains(t, updatedMemo1.Payload.Tags, "shared")

	updatedMemo2, err := ts.Store.GetMemo(ctx, &store.FindMemo{UID: &memo2.UID})
	require.NoError(t, err)
	require.Equal(t, "Second memo also has tag", updatedMemo2.Content)
	require.NotContains(t, updatedMemo2.Payload.Tags, "shared")
}