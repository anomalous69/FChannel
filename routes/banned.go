package routes

import (
	"github.com/anomalous69/fchannel/activitypub"
	"github.com/anomalous69/fchannel/config"
	"github.com/anomalous69/fchannel/db"

	"github.com/anomalous69/fchannel/util"

	"github.com/gofiber/fiber/v2"
)

func BannedGet(ctx *fiber.Ctx) error {

	actor, err := activitypub.GetActorFromDB(config.Domain)

	if err != nil {
		return util.MakeError(err, "BannedGet")
	}

	var data PageData
	data.Name = actor.Name
	data.Boards = activitypub.Boards
	data.Board.Name = ""
	data.Key = config.Key
	data.Board.Domain = config.Domain
	data.Board.ModCred, _ = util.GetPasswordFromSession(ctx)
	data.Board.Actor = actor
	data.Board.Post.Actor = actor.Id
	data.Board.Restricted = actor.Restricted

	data.Meta.Description = data.Name + " is a federated image board based on ActivityPub. The current version of the code running on the server is still a work-in-progress product, expect a bumpy ride for the time being. Get the server code here: https://github.com/anomalous69/FChannel."
	data.Meta.Url = data.Board.Actor.Id
	data.Meta.Title = data.Title

	data.Themes = &config.Themes
	data.ThemeCookie = GetThemeCookie(ctx)

	data.ServerVersion = config.Version

	var banned db.Ban

	banned.IP, banned.Reason, banned.Date, banned.Expires, _ = db.IsIPBanned(ctx.IP())
	if len(banned.IP) > 0 {
		banned.IP = ctx.IP()
	}

	return ctx.Render("banned", fiber.Map{"page": data, "banned": banned}, "layouts/main")
}
