package mapper

import (
	"github.com/gotd/td/tg"

	"go.octolab.org/toolset/indexit/internal/telegram/model"
)

// accountStatus uses only the entity in the current response. In particular,
// a min channel cannot tell us that the account lacks rights or membership.
// Forbidden and missing entities likewise leave the status unknown.
// Monoforums are channel direct-message inboxes, not ordinary memberships;
// their administration depends on the associated channel's permissions.
func accountStatus(chat tg.ChatClass) model.AccountStatus {
	var member, admin, creator bool
	switch c := chat.(type) {
	case *tg.Chat:
		if c == nil {
			return model.AccountStatus{}
		}
		_, hasAdminRights := c.GetAdminRights()
		_, migrated := c.GetMigratedTo()
		member = !c.Left && !c.Deactivated && !migrated
		creator = c.Creator
		admin = creator || hasAdminRights
	case *tg.Channel:
		if c == nil || c.Min || c.Monoforum {
			return model.AccountStatus{}
		}
		_, hasAdminRights := c.GetAdminRights()
		member = !c.Left
		creator = c.Creator
		admin = creator || hasAdminRights
	default:
		return model.AccountStatus{}
	}
	return model.AccountStatus{
		IsMember:  &member,
		IsAdmin:   &admin,
		IsCreator: &creator,
	}
}
