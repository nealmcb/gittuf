// SPDX-License-Identifier: Apache-2.0

package rsl

import (
	"encoding/base64"
	"fmt"
	"testing"

	"github.com/gittuf/gittuf/internal/gitinterface"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/stretchr/testify/assert"
)

const annotationMessage = "test annotation"

func TestNewReferenceEntry(t *testing.T) {
	tempDir := t.TempDir()
	repo := gitinterface.CreateTestGitRepository(t, tempDir)

	if err := NewReferenceEntry("refs/heads/main", gitinterface.ZeroHash).Commit(repo, false); err != nil {
		t.Fatal(err)
	}

	currentTip, err := repo.GetReference(Ref)
	if err != nil {
		t.Fatal(err)
	}

	commitMessage, err := repo.GetCommitMessage(currentTip)
	if err != nil {
		t.Fatal(err)
	}

	parentIDs, err := repo.GetCommitParentIDs(currentTip)
	if err != nil {
		t.Fatal(err)
	}

	expectedMessage := fmt.Sprintf("%s\n\n%s: %s\n%s: %s", ReferenceEntryHeader, RefKey, "refs/heads/main", TargetIDKey, gitinterface.ZeroHash.String())
	assert.Equal(t, expectedMessage, commitMessage)
	assert.Nil(t, parentIDs)

	if err := NewReferenceEntry("refs/heads/main", gitinterface.ZeroHash).Commit(repo, false); err != nil {
		t.Fatal(err)
	}

	newTip, err := repo.GetReference(Ref)
	if err != nil {
		t.Fatal(err)
	}

	commitMessage, err = repo.GetCommitMessage(newTip)
	if err != nil {
		t.Fatal(err)
	}

	parentIDs, err = repo.GetCommitParentIDs(newTip)
	if err != nil {
		t.Fatal(err)
	}

	expectedMessage = fmt.Sprintf("%s\n\n%s: %s\n%s: %s", ReferenceEntryHeader, RefKey, "refs/heads/main", TargetIDKey, gitinterface.ZeroHash.String())
	assert.Equal(t, expectedMessage, commitMessage)
	assert.Contains(t, parentIDs, currentTip)
}

func TestGetLatestEntry(t *testing.T) {
	tempDir := t.TempDir()
	repo := gitinterface.CreateTestGitRepository(t, tempDir)

	if err := NewReferenceEntry("refs/heads/main", gitinterface.ZeroHash).Commit(repo, false); err != nil {
		t.Fatal(err)
	}

	entry, err := GetLatestEntry(repo)
	assert.Nil(t, err)
	e := entry.(*ReferenceEntry)
	assert.Equal(t, "refs/heads/main", e.RefName)
	assert.Equal(t, gitinterface.ZeroHash, e.TargetID)

	if err := NewReferenceEntry("refs/heads/feature", gitinterface.ZeroHash).Commit(repo, false); err != nil {
		t.Fatal(err)
	}

	entry, err = GetLatestEntry(repo)
	assert.Nil(t, err)
	e = entry.(*ReferenceEntry)
	assert.Equal(t, "refs/heads/feature", e.RefName)
	assert.Equal(t, gitinterface.ZeroHash, e.TargetID)

	latestTip, err := repo.GetReference(Ref)
	if err != nil {
		t.Fatal(err)
	}

	if err := NewAnnotationEntry([]gitinterface.Hash{latestTip}, true, "This was a mistaken push!").Commit(repo, false); err != nil {
		t.Error(err)
	}

	entry, err = GetLatestEntry(repo)
	assert.Nil(t, err)
	a := entry.(*AnnotationEntry)
	assert.True(t, a.Skip)
	assert.Equal(t, []gitinterface.Hash{latestTip}, a.RSLEntryIDs)
	assert.Equal(t, "This was a mistaken push!", a.Message)
}

func TestGetLatestNonGittufReferenceEntry(t *testing.T) {
	t.Run("mix of gittuf and non gittuf entries", func(t *testing.T) {
		tempDir := t.TempDir()
		repo := gitinterface.CreateTestGitRepository(t, tempDir)

		// Add the first gittuf entry
		if err := NewReferenceEntry("refs/gittuf/policy", gitinterface.ZeroHash).Commit(repo, false); err != nil {
			t.Fatal(err)
		}

		// Add non gittuf entries
		if err := NewReferenceEntry("refs/heads/main", gitinterface.ZeroHash).Commit(repo, false); err != nil {
			t.Fatal(err)
		}

		// At this point, latest entry should be returned
		expectedLatestEntry, err := GetLatestEntry(repo)
		if err != nil {
			t.Fatal(err)
		}
		latestEntry, annotations, err := GetLatestNonGittufReferenceEntry(repo)
		assert.Nil(t, err)
		assert.Nil(t, annotations)
		assert.Equal(t, expectedLatestEntry, latestEntry)

		// Add another gittuf entry
		if err := NewReferenceEntry("refs/gittuf/not-policy", gitinterface.ZeroHash).Commit(repo, false); err != nil {
			t.Fatal(err)
		}

		// At this point, the expected entry is the same as before
		latestEntry, annotations, err = GetLatestNonGittufReferenceEntry(repo)
		assert.Nil(t, err)
		assert.Nil(t, annotations)
		assert.Equal(t, expectedLatestEntry, latestEntry)

		// Add an annotation for latest entry, check that it's returned
		if err := NewAnnotationEntry([]gitinterface.Hash{expectedLatestEntry.GetID()}, false, annotationMessage).Commit(repo, false); err != nil {
			t.Fatal(err)
		}

		latestEntry, annotations, err = GetLatestNonGittufReferenceEntry(repo)
		assert.Nil(t, err)
		assert.Equal(t, expectedLatestEntry, latestEntry)
		assertAnnotationsReferToEntry(t, latestEntry, annotations)
	})

	t.Run("only gittuf entries", func(t *testing.T) {
		tempDir := t.TempDir()
		repo := gitinterface.CreateTestGitRepository(t, tempDir)

		// Add the first gittuf entry
		if err := NewReferenceEntry("refs/gittuf/policy", gitinterface.ZeroHash).Commit(repo, false); err != nil {
			t.Fatal(err)
		}

		_, _, err := GetLatestNonGittufReferenceEntry(repo)
		assert.ErrorIs(t, err, ErrRSLEntryNotFound)

		// Add another gittuf entry
		if err := NewReferenceEntry("refs/gittuf/not-policy", gitinterface.ZeroHash).Commit(repo, false); err != nil {
			t.Fatal(err)
		}

		_, _, err = GetLatestNonGittufReferenceEntry(repo)
		assert.ErrorIs(t, err, ErrRSLEntryNotFound)
	})
}

func TestGetLatestReferenceEntryForRef(t *testing.T) {
	tempDir := t.TempDir()
	repo := gitinterface.CreateTestGitRepository(t, tempDir)

	refName := "refs/heads/main"
	otherRefName := "refs/heads/feature"

	if err := NewReferenceEntry(refName, gitinterface.ZeroHash).Commit(repo, false); err != nil {
		t.Fatal(err)
	}

	rslRef, err := repo.GetReference(Ref)
	if err != nil {
		t.Fatal(err)
	}

	entry, annotations, err := GetLatestReferenceEntryForRef(repo, refName)
	assert.Nil(t, err)
	assert.Nil(t, annotations)
	assert.Equal(t, rslRef, entry.ID)

	if err := NewReferenceEntry(otherRefName, gitinterface.ZeroHash).Commit(repo, false); err != nil {
		t.Fatal(err)
	}

	entry, annotations, err = GetLatestReferenceEntryForRef(repo, refName)
	assert.Nil(t, err)
	assert.Nil(t, annotations)
	assert.Equal(t, rslRef, entry.ID)

	// Add annotation for the target entry
	if err := NewAnnotationEntry([]gitinterface.Hash{entry.ID}, false, annotationMessage).Commit(repo, false); err != nil {
		t.Fatal(err)
	}

	entry, annotations, err = GetLatestReferenceEntryForRef(repo, refName)
	assert.Nil(t, err)
	assert.Equal(t, rslRef, entry.ID)
	assertAnnotationsReferToEntry(t, entry, annotations)
}

func TestGetLatestReferenceEntryForRefBefore(t *testing.T) {
	t.Run("no annotations", func(t *testing.T) {
		tempDir := t.TempDir()
		repo := gitinterface.CreateTestGitRepository(t, tempDir)

		// RSL structure for the test
		// main <- feature <- main <- feature <- main
		testRefs := []string{"main", "feature", "main", "feature", "main"}
		entryIDs := []gitinterface.Hash{}
		for _, ref := range testRefs {
			if err := NewReferenceEntry(ref, gitinterface.ZeroHash).Commit(repo, false); err != nil {
				t.Fatal(err)
			}
			latest, err := GetLatestEntry(repo)
			if err != nil {
				t.Fatal(err)
			}
			entryIDs = append(entryIDs, latest.GetID())
		}

		entry, annotations, err := GetLatestReferenceEntryForRefBefore(repo, "main", entryIDs[4])
		assert.Nil(t, err)
		assert.Nil(t, annotations)
		assert.Equal(t, entryIDs[2], entry.ID)

		entry, annotations, err = GetLatestReferenceEntryForRefBefore(repo, "main", entryIDs[3])
		assert.Nil(t, err)
		assert.Nil(t, annotations)
		assert.Equal(t, entryIDs[2], entry.ID)

		entry, annotations, err = GetLatestReferenceEntryForRefBefore(repo, "feature", entryIDs[4])
		assert.Nil(t, err)
		assert.Nil(t, annotations)
		assert.Equal(t, entryIDs[3], entry.ID)

		entry, annotations, err = GetLatestReferenceEntryForRefBefore(repo, "feature", entryIDs[3])
		assert.Nil(t, err)
		assert.Nil(t, annotations)
		assert.Equal(t, entryIDs[1], entry.ID)

		_, _, err = GetLatestReferenceEntryForRefBefore(repo, "feature", entryIDs[1])
		assert.ErrorIs(t, err, ErrRSLEntryNotFound)
	})

	t.Run("with annotations", func(t *testing.T) {
		tempDir := t.TempDir()
		repo := gitinterface.CreateTestGitRepository(t, tempDir)

		// RSL structure for the test
		// main <- A <- feature <- A <- main <- A <- feature <- A <- main <- A
		testRefs := []string{"main", "feature", "main", "feature", "main"}
		entryIDs := []gitinterface.Hash{}
		for _, ref := range testRefs {
			if err := NewReferenceEntry(ref, gitinterface.ZeroHash).Commit(repo, false); err != nil {
				t.Fatal(err)
			}
			latest, err := GetLatestEntry(repo)
			if err != nil {
				t.Fatal(err)
			}
			entryIDs = append(entryIDs, latest.GetID())

			if err := NewAnnotationEntry([]gitinterface.Hash{latest.GetID()}, false, annotationMessage).Commit(repo, false); err != nil {
				t.Fatal(err)
			}
			latest, err = GetLatestEntry(repo)
			if err != nil {
				t.Fatal(err)
			}
			entryIDs = append(entryIDs, latest.GetID())
		}

		entry, annotations, err := GetLatestReferenceEntryForRefBefore(repo, "main", entryIDs[4])
		assert.Nil(t, err)
		assert.Equal(t, entryIDs[0], entry.ID)
		assertAnnotationsReferToEntry(t, entry, annotations)
		// Add an annotation at the end for some entry and see it gets pulled in
		// even when the anchor is for its ancestor
		assert.Len(t, annotations, 1) // before adding an annotation, we have just 1
		if err := NewAnnotationEntry([]gitinterface.Hash{entryIDs[0]}, false, annotationMessage).Commit(repo, false); err != nil {
			t.Fatal(err)
		}
		entry, annotations, err = GetLatestReferenceEntryForRefBefore(repo, "main", entryIDs[4])
		assert.Nil(t, err)
		assert.Equal(t, entryIDs[0], entry.ID)
		assertAnnotationsReferToEntry(t, entry, annotations)
		assert.Len(t, annotations, 2) // now we have 2

		entry, annotations, err = GetLatestReferenceEntryForRefBefore(repo, "main", entryIDs[3])
		assert.Nil(t, err)
		assert.Equal(t, entryIDs[0], entry.ID)
		assertAnnotationsReferToEntry(t, entry, annotations)

		entry, annotations, err = GetLatestReferenceEntryForRefBefore(repo, "feature", entryIDs[6])
		assert.Nil(t, err)
		assert.Equal(t, entryIDs[2], entry.ID)
		assertAnnotationsReferToEntry(t, entry, annotations)

		entry, annotations, err = GetLatestReferenceEntryForRefBefore(repo, "feature", entryIDs[7])
		assert.Nil(t, err)
		assert.Equal(t, entryIDs[6], entry.ID)
		assertAnnotationsReferToEntry(t, entry, annotations)

		_, _, err = GetLatestReferenceEntryForRefBefore(repo, "feature", entryIDs[1])
		assert.ErrorIs(t, err, ErrRSLEntryNotFound)
	})
}

func TestGetEntry(t *testing.T) {
	tempDir := t.TempDir()
	repo := gitinterface.CreateTestGitRepository(t, tempDir)

	if err := NewReferenceEntry("main", gitinterface.ZeroHash).Commit(repo, false); err != nil {
		t.Fatal(err)
	}

	initialEntryID, err := repo.GetReference(Ref)
	if err != nil {
		t.Fatal(err)
	}

	if err := NewAnnotationEntry([]gitinterface.Hash{initialEntryID}, true, "This was a mistaken push!").Commit(repo, false); err != nil {
		t.Fatal(err)
	}

	annotationID, err := repo.GetReference(Ref)
	if err != nil {
		t.Fatal(err)
	}

	if err := NewReferenceEntry("main", gitinterface.ZeroHash).Commit(repo, false); err != nil {
		t.Error(err)
	}

	entry, err := GetEntry(repo, initialEntryID)
	assert.Nil(t, err)
	e := entry.(*ReferenceEntry)
	assert.Equal(t, "main", e.RefName)
	assert.Equal(t, gitinterface.ZeroHash, e.TargetID)

	entry, err = GetEntry(repo, annotationID)
	assert.Nil(t, err)
	a := entry.(*AnnotationEntry)
	assert.True(t, a.Skip)
	assert.Equal(t, []gitinterface.Hash{initialEntryID}, a.RSLEntryIDs)
	assert.Equal(t, "This was a mistaken push!", a.Message)
}

func TestGetParentForEntry(t *testing.T) {
	tempDir := t.TempDir()
	repo := gitinterface.CreateTestGitRepository(t, tempDir)

	// Assert no parent for first entry
	if err := NewReferenceEntry("main", gitinterface.ZeroHash).Commit(repo, false); err != nil {
		t.Fatal(err)
	}

	entry, err := GetLatestEntry(repo)
	if err != nil {
		t.Fatal(err)
	}
	entryID := entry.GetID()

	_, err = GetParentForEntry(repo, entry)
	assert.ErrorIs(t, err, ErrRSLEntryNotFound)

	// Find parent for an entry
	if err := NewReferenceEntry("main", gitinterface.ZeroHash).Commit(repo, false); err != nil {
		t.Fatal(err)
	}

	entry, err = GetLatestEntry(repo)
	if err != nil {
		t.Fatal(err)
	}

	parentEntry, err := GetParentForEntry(repo, entry)
	assert.Nil(t, err)
	assert.Equal(t, entryID, parentEntry.GetID())

	entryID = entry.GetID()

	// Find parent for an annotation
	if err := NewAnnotationEntry([]gitinterface.Hash{entryID}, false, annotationMessage).Commit(repo, false); err != nil {
		t.Fatal(err)
	}

	entry, err = GetLatestEntry(repo)
	if err != nil {
		t.Fatal(err)
	}

	parentEntry, err = GetParentForEntry(repo, entry)
	assert.Nil(t, err)
	assert.Equal(t, entryID, parentEntry.GetID())
}

func TestGetNonGittufParentReferenceEntryForEntry(t *testing.T) {
	t.Run("mix of gittuf and non gittuf entries", func(t *testing.T) {
		tempDir := t.TempDir()
		repo := gitinterface.CreateTestGitRepository(t, tempDir)

		// Add the first gittuf entry
		if err := NewReferenceEntry("refs/gittuf/policy", gitinterface.ZeroHash).Commit(repo, false); err != nil {
			t.Fatal(err)
		}

		// Add non gittuf entry
		if err := NewReferenceEntry("refs/heads/main", gitinterface.ZeroHash).Commit(repo, false); err != nil {
			t.Fatal(err)
		}

		expectedEntry, err := GetLatestEntry(repo)
		if err != nil {
			t.Fatal(err)
		}

		// Add non gittuf entry
		if err := NewReferenceEntry("refs/heads/main", gitinterface.ZeroHash).Commit(repo, false); err != nil {
			t.Fatal(err)
		}

		latestEntry, err := GetLatestEntry(repo)
		if err != nil {
			t.Fatal(err)
		}

		parentEntry, annotations, err := GetNonGittufParentReferenceEntryForEntry(repo, latestEntry)
		assert.Nil(t, err)
		assert.Nil(t, annotations)
		assert.Equal(t, expectedEntry, parentEntry)

		// Add another gittuf entry and then a non gittuf entry
		expectedEntry = latestEntry

		if err := NewReferenceEntry("refs/gittuf/not-policy", gitinterface.ZeroHash).Commit(repo, false); err != nil {
			t.Fatal(err)
		}
		if err := NewReferenceEntry("refs/gittuf/main", gitinterface.ZeroHash).Commit(repo, false); err != nil {
			t.Fatal(err)
		}

		latestEntry, err = GetLatestEntry(repo)
		if err != nil {
			t.Fatal(err)
		}

		// The expected entry should be from before this latest gittuf addition
		parentEntry, annotations, err = GetNonGittufParentReferenceEntryForEntry(repo, latestEntry)
		assert.Nil(t, err)
		assert.Nil(t, annotations)
		assert.Equal(t, expectedEntry, parentEntry)

		// Add annotation pertaining to the expected entry
		if err := NewAnnotationEntry([]gitinterface.Hash{expectedEntry.GetID()}, false, annotationMessage).Commit(repo, false); err != nil {
			t.Fatal(err)
		}

		parentEntry, annotations, err = GetNonGittufParentReferenceEntryForEntry(repo, latestEntry)
		assert.Nil(t, err)
		assert.Equal(t, expectedEntry, parentEntry)
		assertAnnotationsReferToEntry(t, parentEntry, annotations)
	})

	t.Run("only gittuf entries", func(t *testing.T) {
		tempDir := t.TempDir()
		repo := gitinterface.CreateTestGitRepository(t, tempDir)

		// Add the first gittuf entry
		if err := NewReferenceEntry("refs/gittuf/policy", gitinterface.ZeroHash).Commit(repo, false); err != nil {
			t.Fatal(err)
		}

		latestEntry, err := GetLatestEntry(repo)
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = GetNonGittufParentReferenceEntryForEntry(repo, latestEntry)
		assert.ErrorIs(t, err, ErrRSLEntryNotFound)

		// Add another gittuf entry
		if err := NewReferenceEntry("refs/gittuf/not-policy", gitinterface.ZeroHash).Commit(repo, false); err != nil {
			t.Fatal(err)
		}

		latestEntry, err = GetLatestEntry(repo)
		if err != nil {
			t.Fatal(err)
		}

		_, _, err = GetNonGittufParentReferenceEntryForEntry(repo, latestEntry)
		assert.ErrorIs(t, err, ErrRSLEntryNotFound)
	})
}

func TestGetFirstEntry(t *testing.T) {
	tempDir := t.TempDir()
	repo := gitinterface.CreateTestGitRepository(t, tempDir)

	if err := NewReferenceEntry("first", gitinterface.ZeroHash).Commit(repo, false); err != nil {
		t.Fatal(err)
	}

	firstEntryT, err := GetLatestEntry(repo)
	if err != nil {
		t.Fatal(err)
	}
	firstEntry := firstEntryT.(*ReferenceEntry)

	for i := 0; i < 5; i++ {
		if err := NewReferenceEntry("main", gitinterface.ZeroHash).Commit(repo, false); err != nil {
			t.Fatal(err)
		}
	}

	testEntry, annotations, err := GetFirstEntry(repo)
	assert.Nil(t, err)
	assert.Nil(t, annotations)
	assert.Equal(t, firstEntry, testEntry)

	for i := 0; i < 5; i++ {
		if err := NewAnnotationEntry([]gitinterface.Hash{firstEntry.ID}, false, annotationMessage).Commit(repo, false); err != nil {
			t.Fatal(err)
		}
	}

	testEntry, annotations, err = GetFirstEntry(repo)
	assert.Nil(t, err)
	assert.Equal(t, firstEntry, testEntry)
	assert.Equal(t, 5, len(annotations))
	assertAnnotationsReferToEntry(t, firstEntry, annotations)
}

func TestGetFirstReferenceEntryForRef(t *testing.T) {
	tempDir := t.TempDir()
	repo := gitinterface.CreateTestGitRepository(t, tempDir)

	if err := NewReferenceEntry("first", gitinterface.ZeroHash).Commit(repo, false); err != nil {
		t.Fatal(err)
	}

	firstEntryT, err := GetLatestEntry(repo)
	if err != nil {
		t.Fatal(err)
	}
	firstEntry := firstEntryT.(*ReferenceEntry)

	for i := 0; i < 5; i++ {
		if err := NewReferenceEntry("main", gitinterface.ZeroHash).Commit(repo, false); err != nil {
			t.Fatal(err)
		}
	}

	testEntry, annotations, err := GetFirstReferenceEntryForRef(repo, "first")
	assert.Nil(t, err)
	assert.Nil(t, annotations)
	assert.Equal(t, firstEntry, testEntry)

	for i := 0; i < 5; i++ {
		if err := NewAnnotationEntry([]gitinterface.Hash{firstEntry.ID}, false, annotationMessage).Commit(repo, false); err != nil {
			t.Fatal(err)
		}
	}

	testEntry, annotations, err = GetFirstReferenceEntryForRef(repo, "first")
	assert.Nil(t, err)
	assert.Equal(t, firstEntry, testEntry)
	assert.Equal(t, 5, len(annotations))
	assertAnnotationsReferToEntry(t, firstEntry, annotations)
}

func TestGetFirstReferenceEntryForCommit(t *testing.T) {
	tempDir := t.TempDir()
	repo := gitinterface.CreateTestGitRepository(t, tempDir)

	treeBuilder := gitinterface.NewReplacementTreeBuilder(repo)
	emptyTreeHash, err := treeBuilder.WriteRootTreeFromBlobIDs(nil)
	if err != nil {
		t.Fatal(err)
	}

	mainRef := "refs/heads/main"

	initialTargetIDs := []gitinterface.Hash{}
	for i := 0; i < 3; i++ {
		commitID, err := repo.Commit(emptyTreeHash, mainRef, "Test commit", false)
		if err != nil {
			t.Fatal(err)
		}

		initialTargetIDs = append(initialTargetIDs, commitID)
	}

	// Right now, the RSL has no entries.
	for _, commitID := range initialTargetIDs {
		_, _, err = GetFirstReferenceEntryForCommit(repo, commitID)
		assert.ErrorIs(t, err, ErrNoRecordOfCommit)
	}

	if err := NewReferenceEntry(mainRef, initialTargetIDs[len(initialTargetIDs)-1]).Commit(repo, false); err != nil {
		t.Fatal(err)
	}

	// At this point, searching for any commit's entry should return the
	// solitary RSL entry.
	latestEntryT, err := GetLatestEntry(repo)
	if err != nil {
		t.Fatal(err)
	}
	for _, commitID := range initialTargetIDs {
		entry, annotations, err := GetFirstReferenceEntryForCommit(repo, commitID)
		assert.Nil(t, err)
		assert.Nil(t, annotations)
		assert.Equal(t, latestEntryT, entry)
	}

	// Now, let's branch off from this ref and add more commits.
	featureRef := "refs/heads/feature"
	// First, "checkout" the feature branch.
	if err := repo.SetReference(featureRef, initialTargetIDs[len(initialTargetIDs)-1]); err != nil {
		t.Fatal(err)
	}

	// Next, add some new commits to this branch.
	featureTargetIDs := []gitinterface.Hash{}
	for i := 0; i < 3; i++ {
		commitID, err := repo.Commit(emptyTreeHash, featureRef, "Feature commit", false)
		if err != nil {
			t.Fatal(err)
		}

		featureTargetIDs = append(featureTargetIDs, commitID)
	}

	// The RSL hasn't seen these new commits, however.
	for _, commitID := range featureTargetIDs {
		_, _, err = GetFirstReferenceEntryForCommit(repo, commitID)
		assert.ErrorIs(t, err, ErrNoRecordOfCommit)
	}

	if err := NewReferenceEntry(featureRef, featureTargetIDs[len(featureTargetIDs)-1]).Commit(repo, false); err != nil {
		t.Fatal(err)
	}

	// At this point, searching for any of the original commits' entry should
	// return the first RSL entry.
	for _, commitID := range initialTargetIDs {
		entry, annotations, err := GetFirstReferenceEntryForCommit(repo, commitID)
		assert.Nil(t, err)
		assert.Nil(t, annotations)
		assert.Equal(t, latestEntryT, entry)
	}
	// Searching for the feature commits should return the second entry.
	latestEntryT, err = GetLatestEntry(repo)
	if err != nil {
		t.Fatal(err)
	}
	for _, commitID := range featureTargetIDs {
		entry, annotations, err := GetFirstReferenceEntryForCommit(repo, commitID)
		assert.Nil(t, err)
		assert.Nil(t, annotations)
		assert.Equal(t, latestEntryT, entry)
	}

	// Now, fast forward main branch to the latest feature branch commit.
	if err := repo.SetReference(mainRef, featureTargetIDs[len(featureTargetIDs)-1]); err != nil {
		t.Fatal(err)
	}

	if err := NewReferenceEntry(mainRef, featureTargetIDs[len(featureTargetIDs)-1]).Commit(repo, false); err != nil {
		t.Fatal(err)
	}

	// Testing for any of the feature commits should return the feature branch
	// entry, not the main branch entry.
	for _, commitID := range featureTargetIDs {
		entry, annotations, err := GetFirstReferenceEntryForCommit(repo, commitID)
		assert.Nil(t, err)
		assert.Nil(t, annotations)
		assert.Equal(t, latestEntryT, entry)
	}

	// Add annotation for feature entry
	if err := NewAnnotationEntry([]gitinterface.Hash{latestEntryT.GetID()}, false, annotationMessage).Commit(repo, false); err != nil {
		t.Fatal(err)
	}

	latestEntry := latestEntryT.(*ReferenceEntry)
	for _, commitID := range featureTargetIDs {
		entry, annotations, err := GetFirstReferenceEntryForCommit(repo, commitID)
		assert.Nil(t, err)
		assert.Equal(t, latestEntryT, entry)
		assertAnnotationsReferToEntry(t, latestEntry, annotations)
	}
}

func TestGetReferenceEntriesInRange(t *testing.T) {
	refName := "refs/heads/main"
	anotherRefName := "refs/heads/feature"

	// We add a mix of reference entries and annotations, establishing expected
	// return values as we go along

	tempDir := t.TempDir()
	repo := gitinterface.CreateTestGitRepository(t, tempDir)

	expectedEntries := []*ReferenceEntry{}
	expectedAnnotationMap := map[gitinterface.Hash][]*AnnotationEntry{}

	// Add some entries to main
	for i := 0; i < 3; i++ {
		if err := NewReferenceEntry(refName, gitinterface.ZeroHash).Commit(repo, false); err != nil {
			t.Fatal(err)
		}

		// We run GetLatestEntry so that the entry has its ID set as well
		entry, err := GetLatestEntry(repo)
		if err != nil {
			t.Fatal(err)
		}
		expectedEntries = append(expectedEntries, entry.(*ReferenceEntry))
	}

	// Add some annotations
	for i := 0; i < 3; i++ {
		if err := NewAnnotationEntry([]gitinterface.Hash{expectedEntries[i].ID}, false, annotationMessage).Commit(repo, false); err != nil {
			t.Fatal(err)
		}

		annotation, err := GetLatestEntry(repo)
		if err != nil {
			t.Fatal(err)
		}
		expectedAnnotationMap[expectedEntries[i].ID] = []*AnnotationEntry{annotation.(*AnnotationEntry)}
	}

	// Each entry has one annotation
	entries, annotationMap, err := GetReferenceEntriesInRange(repo, expectedEntries[0].ID, expectedEntries[len(expectedEntries)-1].ID)
	assert.Nil(t, err)
	assert.Equal(t, expectedEntries, entries)
	assert.Equal(t, expectedAnnotationMap, annotationMap)

	// Add an entry and annotation for feature branch
	if err := NewReferenceEntry(anotherRefName, gitinterface.ZeroHash).Commit(repo, false); err != nil {
		t.Fatal(err)
	}
	latestEntry, err := GetLatestEntry(repo)
	if err != nil {
		t.Fatal(err)
	}
	expectedEntries = append(expectedEntries, latestEntry.(*ReferenceEntry))
	if err := NewAnnotationEntry([]gitinterface.Hash{latestEntry.GetID()}, false, annotationMessage).Commit(repo, false); err != nil {
		t.Fatal(err)
	}
	latestEntry, err = GetLatestEntry(repo)
	if err != nil {
		t.Fatal(err)
	}
	expectedAnnotationMap[expectedEntries[len(expectedEntries)-1].ID] = []*AnnotationEntry{latestEntry.(*AnnotationEntry)}

	// Expected values include the feature branch entry and annotation
	entries, annotationMap, err = GetReferenceEntriesInRange(repo, expectedEntries[0].ID, expectedEntries[len(expectedEntries)-1].ID)
	assert.Nil(t, err)
	assert.Equal(t, expectedEntries, entries)
	assert.Equal(t, expectedAnnotationMap, annotationMap)

	// Add an annotation that refers to two valid entries
	if err := NewAnnotationEntry([]gitinterface.Hash{expectedEntries[0].ID, expectedEntries[1].ID}, false, annotationMessage).Commit(repo, false); err != nil {
		t.Fatal(err)
	}
	latestEntry, err = GetLatestEntry(repo)
	if err != nil {
		t.Fatal(err)
	}
	// This annotation is relevant to both entries
	annotation := latestEntry.(*AnnotationEntry)
	expectedAnnotationMap[expectedEntries[0].ID] = append(expectedAnnotationMap[expectedEntries[0].ID], annotation)
	expectedAnnotationMap[expectedEntries[1].ID] = append(expectedAnnotationMap[expectedEntries[1].ID], annotation)

	entries, annotationMap, err = GetReferenceEntriesInRange(repo, expectedEntries[0].ID, expectedEntries[len(expectedEntries)-1].ID)
	assert.Nil(t, err)
	assert.Equal(t, expectedEntries, entries)
	assert.Equal(t, expectedAnnotationMap, annotationMap)

	// Add a gittuf namespace entry and ensure it's returned as relevant
	if err := NewReferenceEntry("refs/gittuf/relevant", gitinterface.ZeroHash).Commit(repo, false); err != nil {
		t.Fatal(err)
	}
	latestEntry, err = GetLatestEntry(repo)
	if err != nil {
		t.Fatal(err)
	}
	expectedEntries = append(expectedEntries, latestEntry.(*ReferenceEntry))

	entries, annotationMap, err = GetReferenceEntriesInRange(repo, expectedEntries[0].ID, expectedEntries[len(expectedEntries)-1].ID)
	assert.Nil(t, err)
	assert.Equal(t, expectedEntries, entries)
	assert.Equal(t, expectedAnnotationMap, annotationMap)
}

func TestGetReferenceEntriesInRangeForRef(t *testing.T) {
	refName := "refs/heads/main"
	anotherRefName := "refs/heads/feature"

	// We add a mix of reference entries and annotations, establishing expected
	// return values as we go along

	tempDir := t.TempDir()
	repo := gitinterface.CreateTestGitRepository(t, tempDir)

	expectedEntries := []*ReferenceEntry{}
	expectedAnnotationMap := map[gitinterface.Hash][]*AnnotationEntry{}

	// Add some entries to main
	for i := 0; i < 3; i++ {
		if err := NewReferenceEntry(refName, gitinterface.ZeroHash).Commit(repo, false); err != nil {
			t.Fatal(err)
		}

		// We run GetLatestEntry so that the entry has its ID set as well
		entry, err := GetLatestEntry(repo)
		if err != nil {
			t.Fatal(err)
		}
		expectedEntries = append(expectedEntries, entry.(*ReferenceEntry))
	}

	// Add some annotations
	for i := 0; i < 3; i++ {
		if err := NewAnnotationEntry([]gitinterface.Hash{expectedEntries[i].ID}, false, annotationMessage).Commit(repo, false); err != nil {
			t.Fatal(err)
		}

		annotation, err := GetLatestEntry(repo)
		if err != nil {
			t.Fatal(err)
		}
		expectedAnnotationMap[expectedEntries[i].ID] = []*AnnotationEntry{annotation.(*AnnotationEntry)}
	}

	// Each entry has one annotation
	entries, annotationMap, err := GetReferenceEntriesInRangeForRef(repo, expectedEntries[0].ID, expectedEntries[len(expectedEntries)-1].ID, refName)
	assert.Nil(t, err)
	assert.Equal(t, expectedEntries, entries)
	assert.Equal(t, expectedAnnotationMap, annotationMap)

	// Add an entry and annotation for feature branch
	if err := NewReferenceEntry(anotherRefName, gitinterface.ZeroHash).Commit(repo, false); err != nil {
		t.Fatal(err)
	}
	latestEntry, err := GetLatestEntry(repo)
	if err != nil {
		t.Fatal(err)
	}
	if err := NewAnnotationEntry([]gitinterface.Hash{latestEntry.GetID()}, false, annotationMessage).Commit(repo, false); err != nil {
		t.Fatal(err)
	}

	// Expected values do not change
	entries, annotationMap, err = GetReferenceEntriesInRangeForRef(repo, expectedEntries[0].ID, expectedEntries[len(expectedEntries)-1].ID, refName)
	assert.Nil(t, err)
	assert.Equal(t, expectedEntries, entries)
	assert.Equal(t, expectedAnnotationMap, annotationMap)

	// Add an annotation that refers to two valid entries
	if err := NewAnnotationEntry([]gitinterface.Hash{expectedEntries[0].ID, expectedEntries[1].ID}, false, annotationMessage).Commit(repo, false); err != nil {
		t.Fatal(err)
	}
	latestEntry, err = GetLatestEntry(repo)
	if err != nil {
		t.Fatal(err)
	}
	// This annotation is relevant to both entries
	annotation := latestEntry.(*AnnotationEntry)
	expectedAnnotationMap[expectedEntries[0].ID] = append(expectedAnnotationMap[expectedEntries[0].ID], annotation)
	expectedAnnotationMap[expectedEntries[1].ID] = append(expectedAnnotationMap[expectedEntries[1].ID], annotation)

	entries, annotationMap, err = GetReferenceEntriesInRangeForRef(repo, expectedEntries[0].ID, expectedEntries[len(expectedEntries)-1].ID, refName)
	assert.Nil(t, err)
	assert.Equal(t, expectedEntries, entries)
	assert.Equal(t, expectedAnnotationMap, annotationMap)

	// Add a gittuf namespace entry and ensure it's returned as relevant
	if err := NewReferenceEntry("refs/gittuf/relevant", gitinterface.ZeroHash).Commit(repo, false); err != nil {
		t.Fatal(err)
	}
	latestEntry, err = GetLatestEntry(repo)
	if err != nil {
		t.Fatal(err)
	}
	expectedEntries = append(expectedEntries, latestEntry.(*ReferenceEntry))

	entries, annotationMap, err = GetReferenceEntriesInRangeForRef(repo, expectedEntries[0].ID, expectedEntries[len(expectedEntries)-1].ID, refName)
	assert.Nil(t, err)
	assert.Equal(t, expectedEntries, entries)
	assert.Equal(t, expectedAnnotationMap, annotationMap)
}

func TestGetLatestUnskippedReferenceEntryForRef(t *testing.T) {
	refName := "refs/heads/main"

	tempDir := t.TempDir()
	repo := gitinterface.CreateTestGitRepository(t, tempDir)

	entryIDs := []gitinterface.Hash{}

	// Add an entry
	if err := NewReferenceEntry(refName, gitinterface.ZeroHash).Commit(repo, false); err != nil {
		t.Fatal(err)
	}

	// Latest unskipped entry is the one we just added
	e, err := GetLatestEntry(repo)
	if err != nil {
		t.Fatal(err)
	}
	entryIDs = append(entryIDs, e.GetID())

	entry, annotations, err := GetLatestUnskippedReferenceEntryForRef(repo, refName)
	assert.Nil(t, err)
	assert.Empty(t, annotations)
	assert.Equal(t, entryIDs[len(entryIDs)-1], entry.GetID())

	// Add another entry
	if err := NewReferenceEntry(refName, gitinterface.ZeroHash).Commit(repo, false); err != nil {
		t.Fatal(err)
	}

	// Record latest entry's ID
	e, err = GetLatestEntry(repo)
	if err != nil {
		t.Fatal(err)
	}
	entryIDs = append(entryIDs, e.GetID())

	// Latest unskipped entry is the newest one
	entry, annotations, err = GetLatestUnskippedReferenceEntryForRef(repo, refName)
	assert.Nil(t, err)
	assert.Empty(t, annotations)
	assert.Equal(t, entryIDs[len(entryIDs)-1], entry.GetID())

	// Skip the second one
	if err := NewAnnotationEntry([]gitinterface.Hash{entryIDs[1]}, true, "revoke").Commit(repo, false); err != nil {
		t.Fatal(err)
	}

	// Now the latest unskipped entry should be the first one
	entry, annotations, err = GetLatestUnskippedReferenceEntryForRef(repo, refName)
	assert.Nil(t, err)
	assert.Empty(t, annotations)
	assert.Equal(t, entryIDs[0], entry.GetID())

	// Skip the first one too to trigger error
	if err := NewAnnotationEntry([]gitinterface.Hash{entryIDs[0]}, true, "revoke").Commit(repo, false); err != nil {
		t.Fatal(err)
	}

	entry, annotations, err = GetLatestUnskippedReferenceEntryForRef(repo, refName)
	assert.Nil(t, entry)
	assert.Empty(t, annotations)
	assert.ErrorIs(t, err, ErrRSLEntryNotFound)
}

func TestGetLatestUnskippedReferenceEntryForRefBefore(t *testing.T) {
	refName := "refs/heads/main"

	tempDir := t.TempDir()
	repo := gitinterface.CreateTestGitRepository(t, tempDir)

	entryIDs := []gitinterface.Hash{}

	// Add an entry
	if err := NewReferenceEntry(refName, gitinterface.ZeroHash).Commit(repo, false); err != nil {
		t.Fatal(err)
	}

	// Latest unskipped entry is the one we just added
	e, err := GetLatestEntry(repo)
	if err != nil {
		t.Fatal(err)
	}
	entryIDs = append(entryIDs, e.GetID())

	// We use zero hash because we have just the one entry
	entry, annotations, err := GetLatestUnskippedReferenceEntryForRefBefore(repo, refName, gitinterface.ZeroHash)
	assert.Nil(t, err)
	assert.Empty(t, annotations)
	assert.Equal(t, entryIDs[0], entry.GetID())

	// Add another entry
	if err := NewReferenceEntry(refName, gitinterface.ZeroHash).Commit(repo, false); err != nil {
		t.Fatal(err)
	}

	// Record latest entry's ID
	e, err = GetLatestEntry(repo)
	if err != nil {
		t.Fatal(err)
	}
	entryIDs = append(entryIDs, e.GetID())

	// Latest unskipped before the current entry is the first entry
	entry, annotations, err = GetLatestUnskippedReferenceEntryForRefBefore(repo, refName, entryIDs[1])
	assert.Nil(t, err)
	assert.Empty(t, annotations)
	assert.Equal(t, entryIDs[0], entry.GetID())

	// Skip the second one
	if err := NewAnnotationEntry([]gitinterface.Hash{entryIDs[1]}, true, "revoke").Commit(repo, false); err != nil {
		t.Fatal(err)
	}

	// Now even the latest unskipped entry with zero hash should return the first one
	entry, annotations, err = GetLatestUnskippedReferenceEntryForRefBefore(repo, refName, gitinterface.ZeroHash)
	assert.Nil(t, err)
	assert.Empty(t, annotations)
	assert.Equal(t, entryIDs[0], entry.GetID())

	// Skip the first one too to trigger error
	if err := NewAnnotationEntry([]gitinterface.Hash{entryIDs[0]}, true, "revoke").Commit(repo, false); err != nil {
		t.Fatal(err)
	}

	entry, annotations, err = GetLatestUnskippedReferenceEntryForRefBefore(repo, refName, gitinterface.ZeroHash)
	assert.Nil(t, entry)
	assert.Empty(t, annotations)
	assert.ErrorIs(t, err, ErrRSLEntryNotFound)
}

func TestAnnotationEntryRefersTo(t *testing.T) {
	// We use these as stand-ins for actual RSL IDs that have the same data type
	tempDir := t.TempDir()
	repo := gitinterface.CreateTestGitRepository(t, tempDir)

	treeBuilder := gitinterface.NewReplacementTreeBuilder(repo)
	emptyTreeID, err := treeBuilder.WriteRootTreeFromBlobIDs(nil)
	if err != nil {
		t.Fatal(err)
	}

	emptyBlobID, err := repo.WriteBlob(nil)
	if err != nil {
		t.Fatal(err)
	}

	tests := map[string]struct {
		annotation     *AnnotationEntry
		entryID        gitinterface.Hash
		expectedResult bool
	}{
		"annotation refers to single entry, returns true": {
			annotation:     NewAnnotationEntry([]gitinterface.Hash{emptyBlobID}, false, annotationMessage),
			entryID:        emptyBlobID,
			expectedResult: true,
		},
		"annotation refers to multiple entries, returns true": {
			annotation:     NewAnnotationEntry([]gitinterface.Hash{emptyTreeID, emptyBlobID}, false, annotationMessage),
			entryID:        emptyBlobID,
			expectedResult: true,
		},
		"annotation refers to single entry, returns false": {
			annotation:     NewAnnotationEntry([]gitinterface.Hash{emptyBlobID}, false, annotationMessage),
			entryID:        gitinterface.ZeroHash,
			expectedResult: false,
		},
		"annotation refers to multiple entries, returns false": {
			annotation:     NewAnnotationEntry([]gitinterface.Hash{emptyTreeID, emptyBlobID}, false, annotationMessage),
			entryID:        gitinterface.ZeroHash,
			expectedResult: false,
		},
	}

	for name, test := range tests {
		result := test.annotation.RefersTo(test.entryID)
		assert.Equal(t, test.expectedResult, result, fmt.Sprintf("unexpected result in test '%s'", name))
	}
}

func TestReferenceEntryCreateCommitMessage(t *testing.T) {
	nonZeroHash, err := gitinterface.NewHash("abcdef12345678900987654321fedcbaabcdef12")
	if err != nil {
		t.Fatal(err)
	}

	tests := map[string]struct {
		entry           *ReferenceEntry
		expectedMessage string
	}{
		"entry, fully resolved ref": {
			entry: &ReferenceEntry{
				RefName:  "refs/heads/main",
				TargetID: gitinterface.ZeroHash,
			},
			expectedMessage: fmt.Sprintf("%s\n\n%s: %s\n%s: %s", ReferenceEntryHeader, RefKey, "refs/heads/main", TargetIDKey, plumbing.ZeroHash.String()),
		},
		"entry, non-zero commit": {
			entry: &ReferenceEntry{
				RefName:  "refs/heads/main",
				TargetID: nonZeroHash,
			},
			expectedMessage: fmt.Sprintf("%s\n\n%s: %s\n%s: %s", ReferenceEntryHeader, RefKey, "refs/heads/main", TargetIDKey, "abcdef12345678900987654321fedcbaabcdef12"),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			message, _ := test.entry.createCommitMessage()
			if !assert.Equal(t, test.expectedMessage, message) {
				t.Errorf("expected\n%s\n\ngot\n%s", test.expectedMessage, message)
			}
		})
	}
}

func TestAnnotationEntryCreateCommitMessage(t *testing.T) {
	tests := map[string]struct {
		entry           *AnnotationEntry
		expectedMessage string
	}{
		"annotation, no message": {
			entry: &AnnotationEntry{
				RSLEntryIDs: []gitinterface.Hash{gitinterface.ZeroHash},
				Skip:        true,
				Message:     "",
			},
			expectedMessage: fmt.Sprintf("%s\n\n%s: %s\n%s: %s", AnnotationEntryHeader, EntryIDKey, gitinterface.ZeroHash.String(), SkipKey, "true"),
		},
		"annotation, with message": {
			entry: &AnnotationEntry{
				RSLEntryIDs: []gitinterface.Hash{gitinterface.ZeroHash},
				Skip:        true,
				Message:     "message",
			},
			expectedMessage: fmt.Sprintf("%s\n\n%s: %s\n%s: %s\n%s\n%s\n%s", AnnotationEntryHeader, EntryIDKey, gitinterface.ZeroHash.String(), SkipKey, "true", BeginMessage, base64.StdEncoding.EncodeToString([]byte("message")), EndMessage),
		},
		"annotation, with multi-line message": {
			entry: &AnnotationEntry{
				RSLEntryIDs: []gitinterface.Hash{gitinterface.ZeroHash},
				Skip:        true,
				Message:     "message1\nmessage2",
			},
			expectedMessage: fmt.Sprintf("%s\n\n%s: %s\n%s: %s\n%s\n%s\n%s", AnnotationEntryHeader, EntryIDKey, gitinterface.ZeroHash.String(), SkipKey, "true", BeginMessage, base64.StdEncoding.EncodeToString([]byte("message1\nmessage2")), EndMessage),
		},
		"annotation, no message, skip false": {
			entry: &AnnotationEntry{
				RSLEntryIDs: []gitinterface.Hash{gitinterface.ZeroHash},
				Skip:        false,
				Message:     "",
			},
			expectedMessage: fmt.Sprintf("%s\n\n%s: %s\n%s: %s", AnnotationEntryHeader, EntryIDKey, gitinterface.ZeroHash.String(), SkipKey, "false"),
		},
		"annotation, no message, skip false, multiple entry IDs": {
			entry: &AnnotationEntry{
				RSLEntryIDs: []gitinterface.Hash{gitinterface.ZeroHash, gitinterface.ZeroHash},
				Skip:        false,
				Message:     "",
			},
			expectedMessage: fmt.Sprintf("%s\n\n%s: %s\n%s: %s\n%s: %s", AnnotationEntryHeader, EntryIDKey, gitinterface.ZeroHash.String(), EntryIDKey, gitinterface.ZeroHash.String(), SkipKey, "false"),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			message, err := test.entry.createCommitMessage()
			if err != nil {
				t.Fatal(err)
			}
			if !assert.Equal(t, test.expectedMessage, message) {
				t.Errorf("expected\n%s\n\ngot\n%s", test.expectedMessage, message)
			}
		})
	}
}

func TestParseRSLEntryText(t *testing.T) {
	nonZeroHash, err := gitinterface.NewHash("abcdef12345678900987654321fedcbaabcdef12")
	if err != nil {
		t.Fatal(err)
	}

	tests := map[string]struct {
		expectedEntry Entry
		expectedError error
		message       string
	}{
		"entry, fully resolved ref": {
			expectedEntry: &ReferenceEntry{
				ID:       gitinterface.ZeroHash,
				RefName:  "refs/heads/main",
				TargetID: gitinterface.ZeroHash,
			},
			message: fmt.Sprintf("%s\n\n%s: %s\n%s: %s", ReferenceEntryHeader, RefKey, "refs/heads/main", TargetIDKey, gitinterface.ZeroHash.String()),
		},
		"entry, non-zero commit": {
			expectedEntry: &ReferenceEntry{
				ID:       gitinterface.ZeroHash,
				RefName:  "refs/heads/main",
				TargetID: nonZeroHash,
			},
			message: fmt.Sprintf("%s\n\n%s: %s\n%s: %s", ReferenceEntryHeader, RefKey, "refs/heads/main", TargetIDKey, "abcdef12345678900987654321fedcbaabcdef12"),
		},
		"entry, missing header": {
			expectedError: ErrInvalidRSLEntry,
			message:       fmt.Sprintf("%s: %s\n%s: %s", RefKey, "refs/heads/main", TargetIDKey, gitinterface.ZeroHash.String()),
		},
		"entry, missing information": {
			expectedError: ErrInvalidRSLEntry,
			message:       fmt.Sprintf("%s\n\n%s: %s", ReferenceEntryHeader, RefKey, "refs/heads/main"),
		},
		"annotation, no message": {
			expectedEntry: &AnnotationEntry{
				ID:          gitinterface.ZeroHash,
				RSLEntryIDs: []gitinterface.Hash{gitinterface.ZeroHash},
				Skip:        true,
				Message:     "",
			},
			message: fmt.Sprintf("%s\n\n%s: %s\n%s: %s", AnnotationEntryHeader, EntryIDKey, gitinterface.ZeroHash.String(), SkipKey, "true"),
		},
		"annotation, with message": {
			expectedEntry: &AnnotationEntry{
				ID:          gitinterface.ZeroHash,
				RSLEntryIDs: []gitinterface.Hash{gitinterface.ZeroHash},
				Skip:        true,
				Message:     "message",
			},
			message: fmt.Sprintf("%s\n\n%s: %s\n%s: %s\n%s\n%s\n%s", AnnotationEntryHeader, EntryIDKey, gitinterface.ZeroHash.String(), SkipKey, "true", BeginMessage, base64.StdEncoding.EncodeToString([]byte("message")), EndMessage),
		},
		"annotation, with multi-line message": {
			expectedEntry: &AnnotationEntry{
				ID:          gitinterface.ZeroHash,
				RSLEntryIDs: []gitinterface.Hash{gitinterface.ZeroHash},
				Skip:        true,
				Message:     "message1\nmessage2",
			},
			message: fmt.Sprintf("%s\n\n%s: %s\n%s: %s\n%s\n%s\n%s", AnnotationEntryHeader, EntryIDKey, gitinterface.ZeroHash.String(), SkipKey, "true", BeginMessage, base64.StdEncoding.EncodeToString([]byte("message1\nmessage2")), EndMessage),
		},
		"annotation, no message, skip false": {
			expectedEntry: &AnnotationEntry{
				ID:          gitinterface.ZeroHash,
				RSLEntryIDs: []gitinterface.Hash{gitinterface.ZeroHash},
				Skip:        false,
				Message:     "",
			},
			message: fmt.Sprintf("%s\n\n%s: %s\n%s: %s", AnnotationEntryHeader, EntryIDKey, gitinterface.ZeroHash.String(), SkipKey, "false"),
		},
		"annotation, no message, skip false, multiple entry IDs": {
			expectedEntry: &AnnotationEntry{
				ID:          gitinterface.ZeroHash,
				RSLEntryIDs: []gitinterface.Hash{gitinterface.ZeroHash, gitinterface.ZeroHash},
				Skip:        false,
				Message:     "",
			},
			message: fmt.Sprintf("%s\n\n%s: %s\n%s: %s\n%s: %s", AnnotationEntryHeader, EntryIDKey, gitinterface.ZeroHash.String(), EntryIDKey, gitinterface.ZeroHash.String(), SkipKey, "false"),
		},
		"annotation, missing header": {
			expectedError: ErrInvalidRSLEntry,
			message:       fmt.Sprintf("%s: %s\n%s: %s\n%s\n%s\n%s", EntryIDKey, gitinterface.ZeroHash.String(), SkipKey, "true", BeginMessage, base64.StdEncoding.EncodeToString([]byte("message")), EndMessage),
		},
		"annotation, missing information": {
			expectedError: ErrInvalidRSLEntry,
			message:       fmt.Sprintf("%s\n\n%s: %s", AnnotationEntryHeader, EntryIDKey, gitinterface.ZeroHash.String()),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			entry, err := parseRSLEntryText(gitinterface.ZeroHash, test.message)
			if err != nil {
				assert.ErrorIs(t, err, test.expectedError)
			} else if !assert.Equal(t, test.expectedEntry, entry) {
				t.Errorf("expected\n%+v\n\ngot\n%+v", test.expectedEntry, entry)
			}
		})
	}
}

func assertAnnotationsReferToEntry(t *testing.T, entry *ReferenceEntry, annotations []*AnnotationEntry) {
	t.Helper()

	if entry == nil || annotations == nil {
		t.Error("expected entry and annotations, received nil")
	}

	for _, annotation := range annotations {
		assert.True(t, annotation.RefersTo(entry.ID))
		assert.Equal(t, annotationMessage, annotation.Message)
	}
}
