package group

import (
	"github.com/ovh/cds/engine/api/database/gorpmapping"
	"github.com/ovh/cds/engine/gorpmapper"
	"github.com/ovh/cds/sdk"
)

type group struct { // group_authentified_user
	sdk.Group
	gorpmapper.SignedEntity
}

func (g group) Canonical() gorpmapper.CanonicalForms {
	_ = []interface{}{g.ID, g.Name} // Checks that fields exists at compilation
	return []gorpmapper.CanonicalForm{
		"{{printf .ID}}{{.Name}}",
		"{{print .ID}}{{.Name}}",
	}
}

// LinkGroupUser struct for database entity of group_authentified_user table.
type LinkGroupUser struct {
	ID                 int64  `db:"id"`
	GroupID            int64  `db:"group_id"`
	AuthentifiedUserID string `db:"authentified_user_id"`
	Admin              bool   `db:"group_admin"`
	gorpmapper.SignedEntity
}

func (c LinkGroupUser) Canonical() gorpmapper.CanonicalForms {
	_ = []interface{}{c.ID, c.AuthentifiedUserID, c.GroupID, c.Admin} // Checks that fields exists at compilation
	return []gorpmapper.CanonicalForm{
		"{{printf .ID}}{{.AuthentifiedUserID}}{{printf .GroupID}}{{printf .Admin}}",
		"{{print .ID}}{{.AuthentifiedUserID}}{{print .GroupID}}{{print .Admin}}",
	}
}

// LinksGroupUser struct.
type LinksGroupUser []LinkGroupUser

// ToUserIDs returns user ids for given links.
func (l LinksGroupUser) ToUserIDs() []string {
	ids := make([]string, len(l))
	for i := range l {
		ids[i] = l[i].AuthentifiedUserID
	}
	return ids
}

// ToGroupIDs returns group ids for given links.
func (l LinksGroupUser) ToGroupIDs() []int64 {
	ids := make([]int64, len(l))
	for i := range l {
		ids[i] = l[i].GroupID
	}
	return ids
}

// LinkGroupProject struct for database entity of project_group table.
type LinkGroupProject struct {
	gorpmapper.SignedEntity
	ID        int64 `db:"id"`
	GroupID   int64 `db:"group_id"`
	ProjectID int64 `db:"project_id"`
	Role      int   `db:"role"`
	// Aggregates
	Group sdk.Group `db:"-"`
}

func (c LinkGroupProject) Canonical() gorpmapper.CanonicalForms {
	_ = []interface{}{c.ID, c.ProjectID, c.GroupID, c.Role} // Checks that fields exists at compilation
	return []gorpmapper.CanonicalForm{
		"{{printf .ID}}{{printf .ProjectID}}{{printf .GroupID}}{{printf .Role}}",
		"{{print .ID}}{{print .ProjectID}}{{print .GroupID}}{{print .Role}}",
	}
}

// LinksGroupProject struct.
type LinksGroupProject []LinkGroupProject

// ToProjectIDs returns project ids for given links.
func (l LinksGroupProject) ToProjectIDs() []int64 {
	ids := make([]int64, len(l))
	for i := range l {
		ids[i] = l[i].ProjectID
	}
	return ids
}

// ToMapByProjectID groups links by project id in a map.
func (l LinksGroupProject) ToMapByProjectID() map[int64]LinksGroupProject {
	m := make(map[int64]LinksGroupProject)
	for i := range l {
		if _, ok := m[l[i].ProjectID]; !ok {
			m[l[i].ProjectID] = LinksGroupProject{}
		}
		m[l[i].ProjectID] = append(m[l[i].ProjectID], l[i])
	}
	return m
}

type GroupOrganization struct {
	ID             string `db:"id"`
	GroupID        int64  `db:"group_id"`
	OrganizationID string `db:"organization_id"`
	gorpmapper.SignedEntity
}

func (o GroupOrganization) Canonical() gorpmapper.CanonicalForms {
	_ = []interface{}{o.ID, o.GroupID, o.OrganizationID} // Checks that fields exists at compilation
	return []gorpmapper.CanonicalForm{
		"{{printf .ID}}{{printf .GroupID}}{{.OrganizationID}}",
		"{{print .ID}}{{print .GroupID}}{{.OrganizationID}}",
	}
}

func init() {
	gorpmapping.Register(
		gorpmapping.New(group{}, "group", true, "id"),
		gorpmapping.New(LinkGroupUser{}, "group_authentified_user", true, "id"),
		gorpmapping.New(LinkGroupProject{}, "project_group", true, "id"),
		gorpmapping.New(LinkWorkflowGroupPermission{}, "workflow_perm", false),
		gorpmapping.New(GroupOrganization{}, "group_organization", false, "id"),
	)
}
