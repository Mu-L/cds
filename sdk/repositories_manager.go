package sdk

import (
	"time"
)

// RepositoryEvents group all repository events
type RepositoryEvents struct {
	PushEvents        []VCSPushEvent        `json:"push_events" db:"-"`
	CreateEvents      []VCSCreateEvent      `json:"create_events" db:"-"`
	DeleteEvents      []VCSDeleteEvent      `json:"delete_events" db:"-"`
	PullRequestEvents []VCSPullRequestEvent `json:"pullrequest_events" db:"-"`
}

// RepositoryPollerExecution is a polling execution
type RepositoryPollerExecution struct {
	ID                    int64            `json:"id" db:"id"`
	ApplicationID         int64            `json:"-" db:"application_id"`
	PipelineID            int64            `json:"-" db:"pipeline_id"`
	ExecutionPlannedDate  time.Time        `json:"execution_planned_date,omitempty" db:"execution_planned_date"`
	ExecutionDate         *time.Time       `json:"execution_date" db:"execution_date"`
	Executed              bool             `json:"executed" db:"executed"`
	PipelineBuildVersions map[string]int64 `json:"pipeline_build_version" db:"-"`
	Error                 string           `json:"error" db:"error"`
	RepositoryEvents
}

// VCSRelease represents data about release on github, etc..
type VCSRelease struct {
	ID        int64  `json:"id"`
	UploadURL string `json:"upload_url"`
}

// VCSRepo represents data about repository even on stash, or github, etc...
type VCSRepo struct {
	ID              string `json:"id"`
	Name            string `json:"name"`              // On Github: Name = Slug
	Slug            string `json:"slug"`              // On Github: Slug = Name
	Fullname        string `json:"fullname"`          // On Stash : projectkey/slug, on Github : owner/slug
	URL             string `json:"url"`               // Web URL
	URLCommitFormat string `json:"url_commit_format"` // Web URL to commit
	URLBranchFormat string `json:"url_branch_format"` // Web URL to branch
	URLTagFormat    string `json:"url_tag_format"`    // Web URL to tag
	HTTPCloneURL    string `json:"http_url"`          // Git clone URL  "https://<baseURL>/scm/PRJ/my-repo.git"
	SSHCloneURL     string `json:"ssh_url"`           // Git clone URL  "ssh://git@<baseURL>/PRJ/my-repo.git"
}

// VCSAuthor represents the auhor for every commit
type VCSAuthor struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Email       string `json:"emailAddress"`
	Avatar      string `json:"avatar"`
	Slug        string `json:"slug"`
	ID          string `json:"id"`
}

// VCSCommit represents the commit in the repository
type VCSCommit struct {
	Hash      string    `json:"id"`
	Author    VCSAuthor `json:"author"`
	Committer VCSAuthor `json:"committer"`
	Timestamp int64     `json:"authorTimestamp"`
	Message   string    `json:"message"`
	URL       string    `json:"url"`
	Verified  bool      `json:"verified"`
	Signature string    `json:"signature"`
	KeyID     string    `json:"key_id"`
}

// VCSRemote represents remotes known by the repositories manager
type VCSRemote struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// VCSTag represents branches known by the repositories manager
type VCSTag struct {
	Tag       string    `json:"tag"`
	Sha       string    `json:"sha"` // Represent sha of tag
	Message   string    `json:"message"`
	Tagger    VCSAuthor `json:"tagger"`
	Hash      string    `json:"hash"` // Represent hash of commit
	Verified  bool      `json:"verified"`
	Signature string    `json:"signature"`
	KeyID     string    `json:"key_id"`
}

type VCSSearch struct {
}

// VCSBranch represents branches known by the repositories manager
type VCSBranch struct {
	ID           string   `json:"id"`
	DisplayID    string   `json:"display_id"`
	LatestCommit string   `json:"latest_commit"`
	Default      bool     `json:"default"`
	Parents      []string `json:"parents"`
}

type VCSInsight struct {
	Title  string           `json:"title"`
	Detail string           `json:"detail"`
	Datas  []VCSInsightData `json:"data"`
}

type VCSInsightData struct {
	Title string `json:"title"`
	Type  string `json:"type"`
	Text  string `json:"text"`
	Href  string `json:"href"`
}

// VCSPullRequest represents a pull request
type VCSPullRequest struct {
	ID       int          `json:"id"`
	ChangeID string       `json:"change_id"`
	URL      string       `json:"url"`
	User     VCSAuthor    `json:"user"`
	Head     VCSPushEvent `json:"head"`
	Base     VCSPushEvent `json:"base"`
	Title    string       `json:"title"`
	Merged   bool         `json:"merged"`
	Closed   bool         `json:"closed"`
	Revision string       `json:"revision"`
	Updated  time.Time    `json:"updated"`
	MergeBy  VCSAuthor    `json:"merge_by"`
	State    string       `json:"state"`
}

type VCSContent struct {
	Name        string
	IsDirectory bool
	IsFile      bool
	Content     string
}

type VCSPullRequestOptions struct {
	State VCSPullRequestState
}

const (
	VCSPullRequestStateAll    VCSPullRequestState = "all"
	VCSPullRequestStateOpen   VCSPullRequestState = "open"
	VCSPullRequestStateClosed VCSPullRequestState = "closed"
	VCSPullRequestStateMerged VCSPullRequestState = "merged"
)

type VCSPullRequestState string

func (s VCSPullRequestState) IsValid() bool {
	switch s {
	case VCSPullRequestStateAll, VCSPullRequestStateOpen, VCSPullRequestStateClosed, VCSPullRequestStateMerged:
		return true
	}
	return false
}

type VCSPullRequestCommentRequest struct {
	ID       int    `json:"id"`
	ChangeID string `json:"change_id"` // gerrit only
	Revision string `json:"revision"`
	Message  string `json:"message"`
}

// VCSPushEvent represents a push events for polling
type VCSPushEvent struct {
	Repo     string    `json:"repo"`
	Branch   VCSBranch `json:"branch"`
	Commit   VCSCommit `json:"commit"`
	CloneURL string    `json:"clone_url"`
}

// VCSCreateEvent represents a push events for polling
type VCSCreateEvent VCSPushEvent

// VCSDeleteEvent represents a push events for polling
type VCSDeleteEvent struct {
	Branch VCSBranch `json:"branch"`
}

// VCSPullRequestEvent represents a push events for polling
type VCSPullRequestEvent struct {
	Action string       `json:"action"` // opened | closed
	URL    string       `json:"url"`
	Repo   string       `json:"repo"`
	User   VCSAuthor    `json:"user"`
	Head   VCSPushEvent `json:"head"`
	Base   VCSPushEvent `json:"base"`
	Branch VCSBranch    `json:"branch"`
}

// VCSHook represents a hook on a VCS repository
type VCSHook struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Events      []string `json:"events"`
	Method      string   `json:"method"`
	URL         string   `json:"url"`
	ContentType string   `json:"content_type"`
	Body        string   `json:"body"`
	Disable     bool     `json:"disable"`
	InsecureSSL bool     `json:"insecure_ssl"`
}

// VCSCommitStatus represents a status on a VCS repository
type VCSCommitStatus struct {
	Ref        string    `json:"ref"`
	CreatedAt  time.Time `json:"created_at"`
	State      string    `json:"state"`
	Decription string    `json:"description"`
}

func VCSIsSameCommit(sha1, sha1b string) bool {
	if len(sha1) == len(sha1b) {
		return sha1 == sha1b
	}
	if len(sha1) == 12 && len(sha1b) >= 12 {
		return sha1 == sha1b[0:len(sha1)]
	}
	if len(sha1b) == 12 && len(sha1) >= 12 {
		return sha1b == sha1[0:len(sha1b)]
	}
	return false
}
