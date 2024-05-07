// SPDX-License-Identifier: Apache-2.0

package repository

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/gittuf/gittuf/internal/dev"
	"github.com/gittuf/gittuf/internal/gitinterface"
	"github.com/gittuf/gittuf/internal/rsl"
	"github.com/go-git/go-git/v5/plumbing/transport"
)

var (
	ErrCommitNotInRef = errors.New("specified commit is not in ref")
	ErrPushingRSL     = errors.New("unable to push RSL")
	ErrPullingRSL     = errors.New("unable to pull RSL")
)

// RecordRSLEntryForReference is the interface for the user to add an RSL entry
// for the specified Git reference.
func (r *Repository) RecordRSLEntryForReference(refName string, signCommit bool) error {
	slog.Debug("Identifying absolute reference path...")
	absRefName, err := r.r.AbsoluteReference(refName)
	if err != nil {
		return err
	}

	slog.Debug(fmt.Sprintf("Loading current state of '%s'...", absRefName))
	refTip, err := r.r.GetReference(absRefName)
	if err != nil {
		return err
	}

	slog.Debug("Checking for existing entry for reference with same target...")
	isDuplicate, err := r.isDuplicateEntry(absRefName, refTip)
	if err != nil {
		return err
	}
	if isDuplicate {
		return nil
	}

	// TODO: once policy verification is in place, the signing key used by
	// signCommit must be verified for the refName in the delegation tree.

	slog.Debug("Creating RSL reference entry...")
	return rsl.NewReferenceEntry(absRefName, refTip).Commit(r.r, signCommit)
}

// RecordRSLEntryForReferenceAtTarget is a special version of
// RecordRSLEntryForReference used for evaluation. It is only invoked when
// gittuf is explicitly set in developer mode.
func (r *Repository) RecordRSLEntryForReferenceAtTarget(refName string, targetID string, signingKeyBytes []byte) error {
	// Double check that gittuf is in developer mode
	if !dev.InDevMode() {
		return dev.ErrNotInDevMode
	}

	slog.Debug("Identifying absolute reference path...")
	absRefName, err := r.r.AbsoluteReference(refName)
	if err != nil {
		return err
	}

	targetIDHash, err := gitinterface.NewHash(targetID)
	if err != nil {
		return err
	}

	// TODO: once policy verification is in place, the signing key used by
	// signCommit must be verified for the refName in the delegation tree.

	slog.Debug("Creating RSL reference entry...")
	return rsl.NewReferenceEntry(absRefName, targetIDHash).CommitUsingSpecificKey(r.r, signingKeyBytes)
}

// RecordRSLAnnotation is the interface for the user to add an RSL annotation
// for one or more prior RSL entries.
func (r *Repository) RecordRSLAnnotation(rslEntryIDs []string, skip bool, message string, signCommit bool) error {
	rslEntryHashes := []gitinterface.Hash{}
	for _, id := range rslEntryIDs {
		hash, err := gitinterface.NewHash(id)
		if err != nil {
			return err
		}
		rslEntryHashes = append(rslEntryHashes, hash)
	}

	// TODO: once policy verification is in place, the signing key used by
	// signCommit must be verified for the refNames of the rslEntryIDs.

	slog.Debug("Creating RSL annotation entry...")
	return rsl.NewAnnotationEntry(rslEntryHashes, skip, message).Commit(r.r, signCommit)
}

// CheckRemoteRSLForUpdates checks if the RSL at the specified remote
// repository has updated in comparison with the local repository's RSL. This is
// done by fetching the remote RSL to the local repository's remote RSL tracker.
// If the remote RSL has been updated, this method also checks if the local and
// remote RSLs have diverged. In summary, the first return value indicates if
// there is an update and the second return value indicates if the two RSLs have
// diverged and need to be reconciled.
func (r *Repository) CheckRemoteRSLForUpdates(ctx context.Context, remoteName string) (bool, bool, error) {
	trackerRef := rsl.RemoteTrackerRef(remoteName)
	rslRemoteRefSpec := []string{fmt.Sprintf("%s:%s", rsl.Ref, trackerRef)}

	slog.Debug("Updating remote RSL tracker...")
	if err := r.r.FetchRefSpec(remoteName, rslRemoteRefSpec); err != nil {
		if errors.Is(err, transport.ErrEmptyRemoteRepository) {
			// Check if remote is empty and exit appropriately
			return false, false, nil
		}
		return false, false, err
	}

	remoteRefState, err := r.r.GetReference(trackerRef)
	if err != nil {
		return false, false, err
	}

	localRefState, err := r.r.GetReference(rsl.Ref)
	if err != nil {
		return false, false, err
	}

	// Check if local is nil and exit appropriately
	if localRefState.IsZero() {
		// Local RSL has not been populated but remote is not zero
		// So there are updates the local can pull
		slog.Debug("Local RSL has not been initialized but remote RSL exists")
		return true, false, nil
	}

	// Check if equal and exit early if true
	if remoteRefState == localRefState {
		slog.Debug("Local and remote RSLs have same state")
		return false, false, nil
	}

	// Next, check if remote is ahead of local
	knows, err := r.r.KnowsCommit(remoteRefState, localRefState)
	if err != nil {
		return false, false, err
	}
	if knows {
		slog.Debug("Remote RSL is ahead of local RSL")
		return true, false, nil
	}

	// If not ancestor, local may be ahead or they may have diverged
	// If remote is ancestor, only local is ahead, no updates
	// If remote is not ancestor, the two have diverged, local needs to pull updates
	knows, err = r.r.KnowsCommit(localRefState, remoteRefState)
	if err != nil {
		return false, false, err
	}
	if knows {
		slog.Debug("Local RSL is ahead of remote RSL")
		return false, false, nil
	}

	slog.Debug("Local and remote RSLs have diverged")
	return true, true, nil
}

// PushRSL pushes the local RSL to the specified remote. As this push defaults
// to fast-forward only, divergent RSL states are detected.
func (r *Repository) PushRSL(remoteName string) error {
	slog.Debug(fmt.Sprintf("Pushing RSL reference to '%s'...", remoteName))
	if err := r.r.Push(remoteName, []string{rsl.Ref}); err != nil {
		return errors.Join(ErrPushingRSL, err)
	}

	return nil
}

// PullRSL pulls RSL contents from the specified remote to the local RSL. The
// fetch is marked as fast forward only to detect RSL divergence.
func (r *Repository) PullRSL(remoteName string) error {
	slog.Debug(fmt.Sprintf("Pulling RSL reference from '%s'...", remoteName))
	if err := r.r.Fetch(remoteName, []string{rsl.Ref}, true); err != nil {
		return errors.Join(ErrPullingRSL, err)
	}

	return nil
}

// isDuplicateEntry checks if the latest unskipped entry for the ref has the
// same target ID Note that it's legal for the RSL to have target A, then B,
// then A again, this is not considered a duplicate entry
func (r *Repository) isDuplicateEntry(refName string, targetID gitinterface.Hash) (bool, error) {
	latestUnskippedEntry, _, err := rsl.GetLatestUnskippedReferenceEntryForRef(r.r, refName)
	if err != nil {
		if errors.Is(err, rsl.ErrRSLEntryNotFound) {
			return false, nil
		}
		return false, err
	}

	return latestUnskippedEntry.TargetID == targetID, nil
}
