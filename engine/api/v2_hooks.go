package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-gorp/gorp"
	"github.com/gorilla/mux"

	"github.com/ovh/cds/engine/api/database/gorpmapping"
	"github.com/ovh/cds/engine/api/entity"
	"github.com/ovh/cds/engine/api/operation"
	"github.com/ovh/cds/engine/api/project"
	"github.com/ovh/cds/engine/api/repositoriesmanager"
	"github.com/ovh/cds/engine/api/repository"
	"github.com/ovh/cds/engine/api/services"
	"github.com/ovh/cds/engine/api/vcs"
	"github.com/ovh/cds/engine/api/workflow_v2"
	"github.com/ovh/cds/engine/cache"
	"github.com/ovh/cds/engine/service"
	"github.com/ovh/cds/sdk"
	cdslog "github.com/ovh/cds/sdk/log"
	"github.com/rockbears/log"
)

func (api *API) postInsightReportHandler() ([]service.RbacChecker, service.Handler) {
	return service.RBAC(api.isHookService),
		func(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
			vars := mux.Vars(req)
			pkey := vars["projectKey"]
			vcsName := vars["vcsServer"]
			repoName, err := url.PathUnescape(vars["repositoryName"])
			if err != nil {
				return sdk.WithStack(err)
			}
			commit := vars["commit"]
			insightKey := vars["insightKey"]

			var insight sdk.VCSInsight
			if err := service.UnmarshalBody(req, &insight); err != nil {
				return err
			}

			vcsProject, err := api.getVCSByIdentifier(ctx, pkey, vcsName)
			if err != nil {
				return err
			}

			vcsClient, err := repositoriesmanager.AuthorizedClient(ctx, api.mustDB(), api.Cache, pkey, vcsProject.Name)
			if err != nil {
				return err
			}
			if err := vcsClient.CreateInsightReport(ctx, repoName, commit, insightKey, insight); err != nil {
				return err
			}
			return nil
		}
}

func (api *API) postRetrieveEventUserHandler() ([]service.RbacChecker, service.Handler) {
	return service.RBAC(api.isHookService),
		func(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
			var r sdk.HookRetrieveUserRequest
			if err := service.UnmarshalBody(req, &r); err != nil {
				return err
			}
			ctx = context.WithValue(ctx, cdslog.HookEventID, r.HookEventUUID)

			vcsProjectWithSecret, err := vcs.LoadVCSByProject(ctx, api.mustDB(), r.ProjectKey, r.VCSServerName, gorpmapping.GetOptions.WithDecryption)
			if err != nil {
				return err
			}

			resp := sdk.HookRetrieveUserResponse{}
			initiator, _, _, err := findCommitter(ctx, api.Cache, api.mustDB(), r.Commit, r.SignKey, r.ProjectKey, *vcsProjectWithSecret, r.RepositoryName, api.Config.VCS.GPGKeys)
			if err != nil {
				return err
			}
			resp.Initiator = initiator

			log.Debug(ctx, "postRetrieveEventUserHandler:  vcs: %s, repo: %s, commit: %s => intiator: %+v", vcsProjectWithSecret.Name, r.RepositoryName, r.Commit, initiator)

			return service.WriteJSON(w, resp, http.StatusOK)
		}
}

func (api *API) getRetrieveSignKeyOperationHandler() ([]service.RbacChecker, service.Handler) {
	return service.RBAC(api.isHookService),
		func(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
			vars := mux.Vars(req)
			uuid := vars["uuid"]

			ope, err := operation.GetRepositoryOperation(ctx, api.mustDB(), uuid)
			if err != nil {
				return err
			}
			return service.WriteJSON(w, ope, http.StatusOK)
		}
}

func (api *API) postHookEventRetrieveSignKeyHandler() ([]service.RbacChecker, service.Handler) {
	return service.RBAC(api.isHookService),
		func(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
			var hookRetrieveSignKey sdk.HookRetrieveSignKeyRequest
			if err := service.UnmarshalRequest(ctx, req, &hookRetrieveSignKey); err != nil {
				return err
			}

			ctx = context.WithValue(ctx, cdslog.HookEventID, hookRetrieveSignKey.HookEventUUID)
			ctx = context.WithValue(ctx, cdslog.Project, hookRetrieveSignKey.ProjectKey)

			proj, err := project.Load(ctx, api.mustDB(), hookRetrieveSignKey.ProjectKey, project.LoadOptions.WithClearKeys)
			if err != nil {
				return err
			}

			vcsProjectWithSecret, err := vcs.LoadVCSByProject(ctx, api.mustDB(), hookRetrieveSignKey.ProjectKey, hookRetrieveSignKey.VCSServerName, gorpmapping.GetOptions.WithDecryption)
			if err != nil {
				return err
			}

			vcsClient, err := repositoriesmanager.AuthorizedClient(ctx, api.mustDB(), api.Cache, hookRetrieveSignKey.ProjectKey, hookRetrieveSignKey.VCSServerName)
			if err != nil {
				return err
			}
			repo, err := vcsClient.RepoByFullname(ctx, hookRetrieveSignKey.RepositoryName)
			if err != nil {
				log.Info(ctx, "unable to get repository %s/%s for project %s", hookRetrieveSignKey.VCSServerName, hookRetrieveSignKey.RepositoryName, hookRetrieveSignKey.ProjectKey)
				return err
			}

			// Fill ref and commit if empty
			refToClone := hookRetrieveSignKey.Ref
			commit := hookRetrieveSignKey.Commit
			if hookRetrieveSignKey.Ref == "" {
				b, err := vcsClient.Branch(ctx, hookRetrieveSignKey.RepositoryName, sdk.VCSBranchFilters{Default: true})
				if err != nil {
					return err
				}
				refToClone = b.ID
				if commit == "" {
					commit = b.LatestCommit
				}
			} else if commit == "" {
				if strings.HasPrefix(refToClone, sdk.GitRefBranchPrefix) {
					b, err := vcsClient.Branch(ctx, hookRetrieveSignKey.RepositoryName, sdk.VCSBranchFilters{BranchName: strings.TrimPrefix(refToClone, sdk.GitRefBranchPrefix)})
					if err != nil {
						return err
					}
					commit = b.LatestCommit
				} else {
					t, err := vcsClient.Tag(ctx, hookRetrieveSignKey.RepositoryName, strings.TrimPrefix(refToClone, sdk.GitRefTagPrefix))
					if err != nil {
						return err
					}
					commit = t.Hash
				}
			}

			cloneURL := repo.SSHCloneURL
			if vcsProjectWithSecret.Auth.SSHKeyName == "" {
				cloneURL = repo.HTTPCloneURL
			}

			opts := sdk.OperationCheckout{
				Commit:               commit,
				CheckSignature:       hookRetrieveSignKey.GetSigninKey,
				ProcessSemver:        hookRetrieveSignKey.GetSemver,
				GetChangeSet:         hookRetrieveSignKey.GetChangesets,
				ChangeSetCommitSince: hookRetrieveSignKey.ChangesetsCommitSince,
				GetMessage:           hookRetrieveSignKey.GetCommitMessage,
				ChangeSetBranchTo:    hookRetrieveSignKey.ChangesetsBranchTo,
			}
			ope, err := operation.CheckoutAndAnalyzeOperation(ctx, api.mustDB(), *proj, *vcsProjectWithSecret, repo.Fullname, cloneURL, refToClone, opts)
			if err != nil {
				return err
			}

			api.GoRoutines.Exec(context.Background(), "operation-polling-"+ope.UUID, func(ctx context.Context) {
				ope, err := operation.Poll(ctx, api.mustDB(), ope.UUID)
				if err != nil {
					log.ErrorWithStackTrace(ctx, err)
					ope.Status = sdk.OperationStatusError
					ope.Error = &sdk.OperationError{Message: fmt.Sprintf("%v", err)}
				}

				// Send result to hooks
				srvs, err := services.LoadAllByType(ctx, api.mustDB(), sdk.TypeHooks)
				if err != nil {
					log.ErrorWithStackTrace(ctx, err)
					return
				}
				if len(srvs) < 1 {
					log.ErrorWithStackTrace(ctx, sdk.NewErrorFrom(sdk.ErrNotFound, "unable to find hook uservice"))
					return
				}
				callback := sdk.HookEventCallback{
					VCSServerName:      hookRetrieveSignKey.VCSServerName,
					RepositoryName:     hookRetrieveSignKey.RepositoryName,
					HookEventUUID:      hookRetrieveSignKey.HookEventUUID,
					HookEventKey:       hookRetrieveSignKey.HookEventKey,
					SigningKeyCallback: ope,
				}

				if _, code, err := services.NewClient(srvs).DoJSONRequest(ctx, http.MethodPost, "/v2/repository/event/callback", callback, nil); err != nil {
					log.ErrorWithStackTrace(ctx, sdk.WrapError(err, "unable to send analysis call to hook [HTTP: %d]", code))
					return
				}
			})
			return service.WriteJSON(w, ope, http.StatusOK)
		}
}

func (api *API) getV2WorkflowHookHandler() ([]service.RbacChecker, service.Handler) {
	return service.RBAC(api.isHookService),
		func(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
			vars := mux.Vars(req)
			hookID := vars["hookID"]

			h, err := workflow_v2.LoadHooksByID(ctx, api.mustDB(), hookID)
			if err != nil {
				return err
			}
			return service.WriteJSON(w, h, http.StatusOK)
		}
}

func (api *API) postRetrieveWorkflowToTriggerHandler() ([]service.RbacChecker, service.Handler) {
	return service.RBAC(api.isHookService),
		func(ctx context.Context, w http.ResponseWriter, req *http.Request) error {

			var hookRequest sdk.HookListWorkflowRequest
			if err := service.UnmarshalRequest(ctx, req, &hookRequest); err != nil {
				return err
			}

			ctx = context.WithValue(ctx, cdslog.HookEventID, hookRequest.HookEventUUID)

			db := api.mustDB()

			switch hookRequest.RepositoryEventName {
			case sdk.WorkflowHookEventNameWorkflowRun:
				hooks, err := LoadWorkflowHooksWithWorkflowRun(ctx, db, api.Cache, hookRequest)
				if err != nil {
					return err
				}
				return service.WriteJSON(w, hooks, http.StatusOK)
			default:
				uniqueWorkflowMap := make(map[string]struct{})
				filteredWorkflowHooks := make([]sdk.V2WorkflowHook, 0)
				repoCache := make(map[string]string)

				// Get repository web hooks
				workflowHooks, err := LoadWorkflowHooksWithRepositoryWebHooks(ctx, db, api.Cache, hookRequest, repoCache)
				if err != nil {
					return err
				}
				log.Info(ctx, "found %d repository webhooks for event %+v", len(workflowHooks), hookRequest)

				for _, wk := range workflowHooks {
					if _, has := uniqueWorkflowMap[wk.EntityID]; !has {
						filteredWorkflowHooks = append(filteredWorkflowHooks, wk)
						uniqueWorkflowMap[wk.EntityID] = struct{}{}
					}
				}

				// Get workflow_update hooks
				workflowUpdateHooks, err := LoadWorkflowHooksWithWorkflowUpdate(ctx, db, hookRequest)
				if err != nil {
					return err
				}
				log.Info(ctx, "found %d workflow_update hook for event %+v", len(workflowUpdateHooks), hookRequest)
				for _, wk := range workflowUpdateHooks {
					if _, has := uniqueWorkflowMap[wk.EntityID]; !has {
						filteredWorkflowHooks = append(filteredWorkflowHooks, wk)
						uniqueWorkflowMap[wk.EntityID] = struct{}{}
					}
				}

				// Get model_update hooks
				modelUpdateHooks, err := LoadWorkflowHooksWithModelUpdate(ctx, db, hookRequest)
				if err != nil {
					return err
				}
				log.Info(ctx, "found %d workermodel_update hook for event %+v", len(modelUpdateHooks), hookRequest)
				for _, wk := range modelUpdateHooks {
					if _, has := uniqueWorkflowMap[wk.EntityID]; !has {
						filteredWorkflowHooks = append(filteredWorkflowHooks, wk)
						uniqueWorkflowMap[wk.EntityID] = struct{}{}
					}
				}

				hooksWithReadRight := make([]sdk.V2WorkflowHook, 0)
				for _, h := range filteredWorkflowHooks {
					if !hookRequest.AnalyzedProjectKeys.Contains(h.ProjectKey) {
						// Check project right
						vcsClient, err := repositoriesmanager.AuthorizedClient(ctx, db, api.Cache, h.ProjectKey, hookRequest.VCSName)
						if err != nil {
							return err
						}
						if _, err := vcsClient.RepoByFullname(ctx, hookRequest.RepositoryName); err != nil {
							log.Info(ctx, "hook %s of type %s on project %s workflow %s has no right on repository %s/%s: %v", h.ID, h.Type, h.ProjectKey, h.WorkflowName, hookRequest.VCSName, hookRequest.RepositoryName, err)
							continue
						}
					}
					hooksWithReadRight = append(hooksWithReadRight, h)
				}
				return service.WriteJSON(w, hooksWithReadRight, http.StatusOK)
			}
		}
}

// LoadWorkflowHooksWithModelUpdate
// hookRequest contains all updated model from analysis
func LoadWorkflowHooksWithModelUpdate(ctx context.Context, db gorp.SqlExecutor, hookRequest sdk.HookListWorkflowRequest) ([]sdk.V2WorkflowHook, error) {
	filteredWorkflowHooks := make([]sdk.V2WorkflowHook, 0)

	models := make([]string, 0, len(hookRequest.Models))
	for _, m := range hookRequest.Models {
		models = append(models, fmt.Sprintf("%s/%s/%s/%s", m.ProjectKey, m.VCSName, m.RepoName, m.Name))
	}
	entitiesHooks, err := workflow_v2.LoadHooksByModelUpdated(ctx, db, hookRequest.Ref, models)
	if err != nil {
		return nil, err
	}
	filteredWorkflowHooks = append(filteredWorkflowHooks, entitiesHooks...)
	return filteredWorkflowHooks, nil
}

// LoadWorkflowHooksWithWorkflowUpdate
// hookRequest contains all updated workflow from analysis
func LoadWorkflowHooksWithWorkflowUpdate(ctx context.Context, db gorp.SqlExecutor, hookRequest sdk.HookListWorkflowRequest) ([]sdk.V2WorkflowHook, error) {
	filteredWorkflowHooks := make([]sdk.V2WorkflowHook, 0)

	for _, w := range hookRequest.Workflows {
		h, err := workflow_v2.LoadHooksByWorkflowUpdated(ctx, db, w.ProjectKey, w.VCSName, w.RepoName, w.Name, hookRequest.Ref)
		if err != nil {
			if sdk.ErrorIs(err, sdk.ErrNotFound) {
				continue
			}
			return nil, err
		}
		filteredWorkflowHooks = append(filteredWorkflowHooks, *h)
	}
	return filteredWorkflowHooks, nil
}

func LoadWorkflowHooksWithWorkflowRun(ctx context.Context, db gorp.SqlExecutor, cache cache.Store, hookRequest sdk.HookListWorkflowRequest) ([]sdk.V2WorkflowHook, error) {
	wkfName := fmt.Sprintf("%s/%s/%s/%s", hookRequest.Workflows[0].ProjectKey, hookRequest.Workflows[0].VCSName, hookRequest.Workflows[0].RepoName, hookRequest.Workflows[0].Name)
	hooks, err := workflow_v2.LoadHooksWorkflowRunByListeningWorkflow(ctx, db, wkfName)
	if err != nil {
		return nil, err
	}

	// Only gethooks from default branch and head commit
	type branchCache struct {
		Branch string
		Commit string
	}
	repoCache := make(map[string]branchCache)
	vcsClientCache := make(map[string]sdk.VCSAuthorizedClientService)

	filteredHooks := make([]sdk.V2WorkflowHook, 0)
	// Only get hook from default branch + latest commit
	for _, h := range hooks {
		repoCacheKey := fmt.Sprintf("%s/%s", h.VCSName, h.RepositoryName)
		repoData, has := repoCache[repoCacheKey]
		if !has {
			clientCacheKey := h.ProjectKey + "/" + h.VCSName
			client, has := vcsClientCache[clientCacheKey]
			if !has {
				client, err = repositoriesmanager.AuthorizedClient(ctx, db, cache, h.ProjectKey, h.VCSName)
				if err != nil {
					return nil, err
				}
				vcsClientCache[clientCacheKey] = client
			}
			defaultBranch, err := client.Branch(ctx, h.RepositoryName, sdk.VCSBranchFilters{Default: true})
			if err != nil {
				return nil, err
			}
			repoData = branchCache{Branch: defaultBranch.ID, Commit: defaultBranch.LatestCommit}
			repoCache[repoCacheKey] = repoData
		}
		if repoData.Branch != h.Ref || !h.Head {
			continue
		}
		if h.Data.ValidateRef(ctx, hookRequest.Ref) {
			filteredHooks = append(filteredHooks, h)
		}
	}
	return filteredHooks, nil
}

// LoadWorkflowHooksWithRepositoryWebHooks
// If event && workflow declaration are on the same repo : get only the hook defined on the current branch
// Else get all ( analyse process insert only 1 hook for the default branch
func LoadWorkflowHooksWithRepositoryWebHooks(ctx context.Context, db gorp.SqlExecutor, store cache.Store, hookRequest sdk.HookListWorkflowRequest, repoCache map[string]string) ([]sdk.V2WorkflowHook, error) {
	// Repositories hooks
	workflowHooks, err := workflow_v2.LoadHooksByRepositoryEvent(ctx, db, hookRequest.VCSName, hookRequest.RepositoryName, hookRequest.RepositoryEventName)
	if err != nil {
		return nil, err
	}

	filteredWorkflowHooks := make([]sdk.V2WorkflowHook, 0)

	for _, w := range workflowHooks {
		ok, err := validateRepositoryWebHook(ctx, db, store, hookRequest, w, repoCache, false)
		if err != nil {
			return nil, err
		}
		if ok {
			filteredWorkflowHooks = append(filteredWorkflowHooks, w)
		}
	}

	// Check if we skipped a hook that should trigger something
	for _, w := range hookRequest.SkippedHooks {
		if w.Type != sdk.WorkflowHookTypeRepository || w.Data.RepositoryEvent != hookRequest.RepositoryEventName {
			continue
		}
		if w.Data.VCSServer != hookRequest.VCSName || w.Data.RepositoryName != hookRequest.RepositoryName {
			continue
		}
		ok, err := validateRepositoryWebHook(ctx, db, store, hookRequest, w, repoCache, false)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		// If we skipped a hook that should match, fallback on the same REF but on HEAD
		hook, err := workflow_v2.LoadHookHeadRepositoryWebHookByWorkflowAndEvent(ctx, db, w.ProjectKey, w.VCSName, w.RepositoryName, w.WorkflowName, w.Data.RepositoryEvent, w.Ref)
		if err != nil {
			if !sdk.ErrorIs(err, sdk.ErrNotFound) {
				return nil, err
			}

			// If we found nothing, fallback on default branch
			defaultBranch, has := repoCache[w.VCSName+"/"+w.RepositoryName]
			if !has {
				// Fallback on default branch
				vcsClient, err := repositoriesmanager.AuthorizedClient(ctx, db, store, w.ProjectKey, w.VCSName)
				if err != nil {
					return nil, err
				}
				b, err := vcsClient.Branch(ctx, w.RepositoryName, sdk.VCSBranchFilters{Default: true})
				if err != nil {
					return nil, err
				}
				defaultBranch = b.ID
				repoCache[w.VCSName+"/"+w.RepositoryName] = b.ID
			}

			hook, err = workflow_v2.LoadHookHeadRepositoryWebHookByWorkflowAndEvent(ctx, db, w.ProjectKey, w.VCSName, w.RepositoryName, w.WorkflowName, w.Data.RepositoryEvent, defaultBranch)
			if err != nil {
				if !sdk.ErrorIs(err, sdk.ErrNotFound) {
					return nil, err
				}
				continue
			}
		}
		if hook == nil {
			continue
		}
		ok, err = validateRepositoryWebHook(ctx, db, store, hookRequest, *hook, repoCache, true)
		if err != nil {
			return nil, err
		}
		if ok {
			// Load entity to get the right commit instead of HEAD
			if hook.Commit == "HEAD" {
				e, err := entity.LoadByID(ctx, db, hook.EntityID)
				if err != nil {
					return nil, err
				}
				hook.Commit = e.Commit
			}
			filteredWorkflowHooks = append(filteredWorkflowHooks, *hook)
		}
	}

	// For PullRequest event, skipped hooks are always empty
	if len(hookRequest.SkippedHooks) == 0 && hookRequest.RepositoryEventName == sdk.WorkflowHookEventNamePullRequest {

		hooks, err := workflow_v2.LoadHookHeadPullRequestHookByWorkflowAndEvent(ctx, db, hookRequest.VCSName, hookRequest.RepositoryName)
		if err != nil {
			return nil, err
		}
		headhooks := make(map[string]sdk.V2WorkflowHook)
		for _, h := range hooks {
			// Check with existing hook to trigger
			skip := false
			for _, filteredHook := range filteredWorkflowHooks {
				// Ignore if it cames from the same entity
				if filteredHook.EntityID == h.EntityID {
					skip = true
					break
				}
				// Ignore if the workflow is already trigger
				if h.VCSName == filteredHook.VCSName && h.RepositoryName == filteredHook.RepositoryName && h.WorkflowName == filteredHook.WorkflowName {
					skip = true
					break
				}
			}
			if skip {
				continue
			}

			hookKey := fmt.Sprintf("%s/%s/%s", h.VCSName, h.RepositoryName, h.WorkflowName)

			// If definition found is on the same repo / branch, add the hook
			if h.VCSName == hookRequest.VCSName && h.RepositoryName == hookRequest.RepositoryName && h.Ref == hookRequest.Ref {
				headhooks[hookKey] = h
				continue
			}
			// If definition found is on the same repo but another branch: check the ref
			if h.VCSName == hookRequest.VCSName && h.RepositoryName == hookRequest.RepositoryName && h.Ref != hookRequest.Ref {
				_, has := headhooks[hookKey]
				if !has {
					// Check default branch
					vcsAuth, err := repositoriesmanager.AuthorizedClient(ctx, db, store, h.ProjectKey, h.VCSName)
					if err != nil {
						return nil, err
					}
					b, err := vcsAuth.Branch(ctx, h.RepositoryName, sdk.VCSBranchFilters{Default: true})
					if err != nil {
						return nil, err
					}
					// If default branch, keep it
					if h.Ref == b.ID {
						headhooks[hookKey] = h
					}
				}
				continue
			}
			if h.VCSName != hookRequest.VCSName || h.RepositoryName != hookRequest.RepositoryName {
				// Add it. On distant workflow, only the webhook on default branch is saved
				headhooks[hookKey] = h
				continue
			}
		}

		for _, hook := range headhooks {
			ok, err := validateRepositoryWebHook(ctx, db, store, hookRequest, hook, repoCache, true)
			if err != nil {
				return nil, err
			}
			if ok {
				// Load entity to get the right commit instead of HEAD
				if hook.Commit == "HEAD" {
					e, err := entity.LoadByID(ctx, db, hook.EntityID)
					if err != nil {
						return nil, err
					}
					hook.Commit = e.Commit
				}
				filteredWorkflowHooks = append(filteredWorkflowHooks, hook)
			}
		}
	}

	return filteredWorkflowHooks, nil
}

func validateRepositoryWebHook(ctx context.Context, db gorp.SqlExecutor, store cache.Store, hookRequest sdk.HookListWorkflowRequest, w sdk.V2WorkflowHook, repoCache map[string]string, fallback bool) (bool, error) {
	// Skip branch/commit validation for fallback
	if !fallback {
		// If event && workflow declaration are on the same repo
		if w.VCSName == hookRequest.VCSName && w.RepositoryName == hookRequest.RepositoryName {
			// Only get workflow configuration from current branch/commit
			if w.Ref != hookRequest.Ref || w.Commit != hookRequest.Sha {
				return false, nil
			}
		} else {
			// For distant workflow, only allow default branch hook with head = true
			defaultBranch, has := repoCache[w.VCSName+"/"+w.RepositoryName]
			if !has {
				vcsAuth, err := repositoriesmanager.AuthorizedClient(ctx, db, store, w.ProjectKey, w.VCSName)
				if err != nil {
					return false, err
				}
				b, err := vcsAuth.Branch(ctx, w.RepositoryName, sdk.VCSBranchFilters{Default: true})
				if err != nil {
					return false, err
				}
				repoCache[w.VCSName+"/"+w.RepositoryName] = b.ID
				defaultBranch = b.ID
			}
			if w.Ref != defaultBranch || !w.Head {
				return false, nil
			}
		}
	}

	// Check configuration : branch filter + path filter
	switch hookRequest.RepositoryEventName {
	case sdk.WorkflowHookEventNamePush:
		return w.Data.ValidateRef(ctx, hookRequest.Ref), nil
	case sdk.WorkflowHookEventNamePullRequest, sdk.WorkflowHookEventNamePullRequestComment:
		validType := true
		if len(w.Data.TypesFilter) > 0 {
			validType = sdk.IsInArray(hookRequest.RepositoryEventType, w.Data.TypesFilter)
		}
		return w.Data.ValidateRef(ctx, hookRequest.PullRequestRefTo) && validType, nil
	}
	return false, nil
}

func (api *API) getHooksRepositoriesHandler() ([]service.RbacChecker, service.Handler) {
	return service.RBAC(api.isHookService),
		func(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
			vars := mux.Vars(req)
			vcsName := vars["vcsServer"]

			repoName, err := url.PathUnescape(vars["repositoryName"])
			if err != nil {
				return sdk.WithStack(err)
			}

			repos, err := repository.LoadByNameWithoutVCSServer(ctx, api.mustDB(), repoName)
			if err != nil {
				return err
			}

			repositories := make([]sdk.ProjectRepository, 0)
			for _, r := range repos {
				vcsserver, err := vcs.LoadVCSByIDAndProjectKey(ctx, api.mustDB(), r.ProjectKey, r.VCSProjectID)
				if err != nil {
					return err
				}
				if vcsserver.Name == vcsName {
					repositories = append(repositories, r)
				}
			}
			return service.WriteJSON(w, repositories, http.StatusOK)
		}
}
