// Copyright 2023 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"testing"

	"gitea.dev/models/db"
	"gitea.dev/models/unittest"
	"gitea.dev/modules/optional"
	"gitea.dev/modules/util"

	"github.com/stretchr/testify/assert"
)

func TestMigrate_InsertReleases(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	a := &Attachment{
		UUID: "a0eebc91-9c0c-4ef7-bb6e-6bb9bd380a12",
	}
	r := &Release{
		Attachments: []*Attachment{a},
	}

	err := InsertReleases(t.Context(), r)
	assert.NoError(t, err)
}

func Test_FindTagsByCommitIDs(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	sha1Rels, err := FindTagsByCommitIDs(t.Context(), 1, "65f1bf27bc3bf70f64657658635e66094edbcb4d")
	assert.NoError(t, err)
	assert.Len(t, sha1Rels, 1)
	rels := sha1Rels["65f1bf27bc3bf70f64657658635e66094edbcb4d"]
	assert.Len(t, rels, 3)
	assert.Equal(t, "v1.1", rels[0].TagName)
	assert.Equal(t, "delete-tag", rels[1].TagName)
	assert.Equal(t, "v1.0", rels[2].TagName)
}

func TestFindReleasesAttachmentNamePattern(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	// repo 1: release 1 has "attach1", draft release 4 has "draft-attach"
	find := func(pattern string, includeDrafts bool) []*Release {
		rels, err := db.Find[Release](t.Context(), FindReleasesOptions{
			ListOptions:           db.ListOptionsAll,
			RepoID:                1,
			IncludeDrafts:         includeDrafts,
			AttachmentNamePattern: optional.Some(pattern),
		})
		assert.NoError(t, err)
		return rels
	}

	rels := find("ATTACH", true)
	assert.Len(t, rels, 2, "case-insensitive substring match")
	assert.ElementsMatch(t, []int64{1, 4}, []int64{rels[0].ID, rels[1].ID})

	assert.Empty(t, find("draft-attach", false), "draft visibility is still controlled by IncludeDrafts")
	assert.Empty(t, find("readme", true), "attachments of other repos must not match")
	assert.Empty(t, find("%", true), "LIKE wildcards must be treated as literal text")

	// two matching attachments on one release must return the release only once
	assert.NoError(t, db.Insert(t.Context(), &Attachment{
		UUID:      "a0eebc99-9c0c-4ef7-bb6e-6bb9bd380a99",
		RepoID:    1,
		ReleaseID: 1,
		Name:      "second-attach.zip",
	}))
	rels = find("attach", true)
	assert.Len(t, rels, 2)
	assert.ElementsMatch(t, []int64{1, 4}, []int64{rels[0].ID, rels[1].ID})

	count, err := db.Count[Release](t.Context(), FindReleasesOptions{
		RepoID:                1,
		IncludeDrafts:         true,
		AttachmentNamePattern: optional.Some("attach"),
	})
	assert.NoError(t, err)
	assert.EqualValues(t, 2, count, "count must match the filtered releases for paging")
}

func TestAddReleaseAttachmentsRejectsDifferentRepo(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	uuid := "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a12" // attachment 2 belongs to repo 2
	err := AddReleaseAttachments(t.Context(), 1, []string{uuid})
	assert.Error(t, err)
	assert.ErrorIs(t, err, util.ErrPermissionDenied)

	attach, err := GetAttachmentByUUID(t.Context(), uuid)
	assert.NoError(t, err)
	assert.Zero(t, attach.ReleaseID, "attachment should not be linked to release on failure")
}

func TestAddReleaseAttachmentsAllowsLegacyMissingRepoID(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	legacyUUID := "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a20" // attachment 10 has repo_id 0
	err := AddReleaseAttachments(t.Context(), 1, []string{legacyUUID})
	assert.NoError(t, err)

	attach, err := GetAttachmentByUUID(t.Context(), legacyUUID)
	assert.NoError(t, err)
	assert.EqualValues(t, 1, attach.RepoID)
	assert.EqualValues(t, 1, attach.ReleaseID)
}

func TestAddReleaseAttachmentsRejectsRecentZeroRepoID(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	recentUUID := "a0eebc99-9c0b-4ef8-bb6d-6bb9bd3800aa"
	attachment := &Attachment{
		UUID:        recentUUID,
		RepoID:      0,
		IssueID:     0,
		ReleaseID:   0,
		CommentID:   0,
		Name:        "recent-zero",
		CreatedUnix: LegacyAttachmentMissingRepoIDCutoff + 1,
	}
	assert.NoError(t, db.Insert(t.Context(), attachment))

	err := AddReleaseAttachments(t.Context(), 1, []string{recentUUID})
	assert.Error(t, err)
	assert.ErrorIs(t, err, util.ErrPermissionDenied)

	attach, err := GetAttachmentByUUID(t.Context(), recentUUID)
	assert.NoError(t, err)
	assert.Zero(t, attach.ReleaseID)
	assert.Zero(t, attach.RepoID)
}
