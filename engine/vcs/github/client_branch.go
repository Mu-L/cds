package github

import (
	"context"
	"fmt"
	"net/http"

	"github.com/rockbears/log"

	"github.com/ovh/cds/engine/cache"
	"github.com/ovh/cds/sdk"
)

// Branches returns list of branches for a repo
// https://developer.github.com/v3/repos/branches/#list-branches
func (g *githubClient) Branches(ctx context.Context, fullname string, filters sdk.VCSBranchesFilter) ([]sdk.VCSBranch, error) {
	var branches = []Branch{}
	repo, err := g.repoByFullname(ctx, fullname)
	if err != nil {
		return nil, err
	}
	var noEtag bool
	var attempt int

	var nextPage = "/repos/" + fullname + "/branches"
	for nextPage != "" {
		if ctx.Err() != nil {
			break
		}
		if filters.Limit > 0 && len(branches) >= int(filters.Limit) {
			break
		}

		var opt getArgFunc
		if noEtag {
			opt = withoutETag
		} else {
			opt = withETag
		}

		attempt++
		status, body, headers, err := g.get(ctx, nextPage, opt)
		if err != nil {
			log.Warn(ctx, "githubClient.Branches> Error %s", err)
			return nil, err
		}
		if status >= 400 {
			return nil, sdk.NewError(sdk.ErrUnknownError, errorAPI(body))
		}
		nextBranches := []Branch{}

		//Github may return 304 status because we are using conditional request with ETag based headers
		if status == http.StatusNotModified {
			//If repos aren't updated, lets get them from cache
			k := cache.Key("vcs", "github", "branches", sdk.Hash512(g.OAuthToken+g.username), "/repos/"+fullname+"/branches")
			_, err := g.Cache.Get(k, &branches)
			if err != nil {
				log.Error(ctx, "cannot get from cache %s: %v", k, err)
			}
			if len(branches) != 0 || attempt > 5 {
				//We found branches, let's exit the loop
				break
			}
			//If we did not found any branch in cache, let's retry (same nextPage) without etag
			noEtag = true
			continue
		} else {
			if err := sdk.JSONUnmarshal(body, &nextBranches); err != nil {
				log.Warn(ctx, "githubClient.Branches> Unable to parse github branches: %s", err)
				return nil, err
			}
		}

		branches = append(branches, nextBranches...)
		nextPage = getNextPage(headers)
	}

	//Put the body on cache for one hour and one minute
	k := cache.Key("vcs", "github", "branches", sdk.Hash512(g.OAuthToken+g.username), "/repos/"+fullname+"/branches")
	if err := g.Cache.SetWithTTL(k, branches, 61*60); err != nil {
		log.Error(ctx, "cannot SetWithTTL: %s: %v", k, err)
	}

	defaultBranchFound := false
	branchesResult := []sdk.VCSBranch{}
	for _, b := range branches {
		branch := sdk.VCSBranch{
			DisplayID:    b.Name,
			ID:           "refs/heads/" + b.Name,
			LatestCommit: b.Commit.Sha,
			Default:      b.Name == repo.DefaultBranch,
		}
		for _, p := range b.Commit.Parents {
			branch.Parents = append(branch.Parents, p.Sha)
		}
		branchesResult = append(branchesResult, branch)

		if branch.Default {
			defaultBranchFound = true
		}
	}

	if !defaultBranchFound {
		b, err := g.Branch(ctx, fullname, sdk.VCSBranchFilters{Default: true})
		if err != nil {
			return nil, err
		}
		branchesResult = append(branchesResult, *b)
	}

	return branchesResult, nil
}

// Branch returns only detail of a branch
func (g *githubClient) Branch(ctx context.Context, fullname string, filters sdk.VCSBranchFilters) (*sdk.VCSBranch, error) {
	if filters.Default {
		repo, err := g.repoByFullname(ctx, fullname)
		if err != nil {
			return nil, err
		}
		filters.BranchName = repo.DefaultBranch
	}

	cacheBranchKey := cache.Key("vcs", "github", "branches", sdk.Hash512(g.OAuthToken+g.username), "/repos/"+fullname+"/branch/"+filters.BranchName)
	repo, err := g.repoByFullname(ctx, fullname)
	if err != nil {
		return nil, err
	}

	url := "/repos/" + fullname + "/branches/" + filters.BranchName
	status, body, _, err := g.get(ctx, url)
	if err != nil {
		if err := g.Cache.Delete(cacheBranchKey); err != nil {
			log.Error(ctx, "githubClient.Branch> unable to delete cache key %v: %v", cacheBranchKey, err)
		}
		return nil, err
	}
	if status == 404 {
		return nil, sdk.WithStack(sdk.ErrNoBranch)
	}
	if status >= 400 {
		if err := g.Cache.Delete(cacheBranchKey); err != nil {
			log.Error(ctx, "githubClient.Branch> unable to delete cache key %v: %v", cacheBranchKey, err)
		}
		return nil, sdk.NewError(sdk.ErrUnknownError, errorAPI(body))
	}

	//Github may return 304 status because we are using conditional request with ETag based headers
	var branch Branch
	if status == http.StatusNotModified {
		//If repos aren't updated, lets get them from cache
		find, err := g.Cache.Get(cacheBranchKey, &branch)
		if err != nil {
			log.Error(ctx, "cannot get from cache %s: %v", cacheBranchKey, err)
		}
		if !find {
			log.Error(ctx, "Unable to get branch (%s) from the cache", cacheBranchKey)
		}
	} else {
		if err := sdk.JSONUnmarshal(body, &branch); err != nil {
			log.Warn(ctx, "githubClient.Branch> Unable to parse github branch: %s", err)
			return nil, err
		}
	}

	if branch.Name == "" {
		log.Warn(ctx, "githubClient.Branch> Cannot find branch %v: %s", branch, filters.BranchName)
		if err := g.Cache.Delete(cacheBranchKey); err != nil {
			log.Error(ctx, "githubClient.Branch> unable to delete cache key %v: %v", cacheBranchKey, err)
		}
		return nil, fmt.Errorf("githubClient.Branch > Cannot find branch %s", filters.BranchName)
	}

	//Put the body on cache for one hour and one minute
	k := cache.Key("vcs", "github", "branches", sdk.Hash512(g.OAuthToken+g.username), "/repos/"+fullname+"/branch/"+filters.BranchName)
	if err := g.Cache.SetWithTTL(k, branch, 61*60); err != nil {
		log.Error(ctx, "cannot SetWithTTL: %s: %v", k, err)
	}

	branchResult := &sdk.VCSBranch{
		DisplayID:    branch.Name,
		ID:           "refs/heads/" + branch.Name,
		LatestCommit: branch.Commit.Sha,
		Default:      branch.Name == repo.DefaultBranch,
	}

	if branch.Commit.Sha != "" {
		for _, p := range branch.Commit.Parents {
			branchResult.Parents = append(branchResult.Parents, p.Sha)
		}
	}

	return branchResult, nil
}
