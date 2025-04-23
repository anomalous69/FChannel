package routes

import (
	"github.com/anomalous69/fchannel/activitypub"
	"github.com/anomalous69/fchannel/config"
	"github.com/anomalous69/fchannel/db"
	"github.com/anomalous69/fchannel/util"
	"github.com/gofiber/fiber/v2"
)

func Index(ctx *fiber.Ctx) error {
	actor, err := activitypub.GetActorFromDB(config.Domain)
	if err != nil {
		return util.MakeError(err, "Index")
	}

	// this is a activitpub json request return json instead of html page
	if activitypub.AcceptActivity(ctx.Get("Accept")) {
		actor.GetInfoResp(ctx)
		return nil
	}

	var data PageData

	data.NewsItems, err = db.GetNews(3)
	if err != nil {
		return util.MakeError(err, "Index")
	}

	collection, err := actor.GetRecentThreads()

	if err != nil {
		return util.MakeError(err, "Index")
	}

	data.Title = "Welcome to " + actor.Name
	data.Name = actor.Name
	data.Boards = activitypub.Boards
	data.Posts = collection.OrderedItems
	data.Board.Name = ""
	data.Key = config.Key
	data.Board.Domain = config.Domain
	data.Board.ModCred, _ = util.GetPasswordFromSession(ctx)
	data.Board.Actor = actor
	data.Board.Post.Actor = actor.Id
	data.Board.Restricted = actor.Restricted
	//almost certainly there is a better algorithm for this but the old one was wrong
	//and I suck at math. This works at least.
	data.BoardRemainer = make([]int, 3-(len(data.Boards)%3))

	if len(data.BoardRemainer) == 3 {
		data.BoardRemainer = make([]int, 0)
	}

	data.Meta.Description = data.Name + " a federated image board based on ActivityPub. The current version of the code running on the server is still a work-in-progress product, expect a bumpy ride for the time being. Get the server code here: https://github.com/anomalous69/FChannel."
	data.Meta.Url = data.Board.Domain
	data.Meta.Title = data.Title

	data.Themes = &config.Themes
	data.ThemeCookie = GetThemeCookie(ctx)

	data.ServerVersion = config.Version

	return ctx.Render("index", fiber.Map{
		"page": data,
	}, "layouts/main")
}

func Inbox(ctx *fiber.Ctx) error {
	// TODO main actor Inbox route
	return ctx.SendString("main inbox")
}

func Outbox(ctx *fiber.Ctx) error {
	actor, err := activitypub.GetActorFromPath(ctx.Path(), "/")

	if err != nil {
		return util.MakeError(err, "Outbox")
	}

	if activitypub.AcceptActivity(ctx.Get("Accept")) {
		actor.GetOutbox(ctx)
		return nil
	}

	return ParseOutboxRequest(ctx, actor)
}

func Following(ctx *fiber.Ctx) error {
	actor, _ := activitypub.GetActorFromDB(config.Domain)
	return actor.GetFollowingResp(ctx)
}

func Followers(ctx *fiber.Ctx) error {
	actor, _ := activitypub.GetActorFromDB(config.Domain)
	return actor.GetFollowersResp(ctx)
}
