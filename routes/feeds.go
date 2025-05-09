package routes

import (
	"database/sql"
	"strconv"
	"time"

	"github.com/anomalous69/fchannel/activitypub"
	"github.com/anomalous69/fchannel/config"
	"github.com/anomalous69/fchannel/db"
	"github.com/anomalous69/fchannel/util"
	"github.com/gofiber/fiber/v2"
	"github.com/gorilla/feeds"
)

func GetThreadFeed(ctx *fiber.Ctx) error {
	actor, err := activitypub.GetActorFromDB(config.Domain + "/" + ctx.Params("actor"))
	if err != nil || actor.Id == "" {
		return Send404(ctx, "Board /"+ctx.Params("actor")+"/ does not exist")
	}
	thread, err := db.GetPostIDFromNum(ctx.Params("post"))
	if err != nil || thread == "" {
		return Send404(ctx, "Thread "+ctx.Params("post")+" does not exist")
	}

	if !db.IsValidThread(thread) {
		return Send404(ctx, "Thread "+ctx.Params("post")+" does not exist")
	}
	feedtype := ctx.Params("feedtype")

	limit, err := strconv.Atoi(ctx.Query("limit"))
	if err != nil {
		limit = 100 // default limit or handle the error appropriately
	}

	if ctx.Query("limit") == "0" {
		limit = 999999999
	}

	now := time.Now()
	feed := &feeds.Feed{
		Title:   "/" + actor.PreferredUsername + "/ - " + ctx.Params("post"), //TODO: Put thread subject here if it exists
		Link:    &feeds.Link{Href: thread},
		Created: now,
	}

	var rows *sql.Rows

	query := `select x.id, x.name, x.content, x.published, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (select id, name, content, published,
			attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where id in (select id from replies where inreplyto = $1) and type='Note' union select id, name, content, published,
			attributedto, attachment, preview, actor, tripcode, sensitive from cacheactivitystream where id in (select id from replies where inreplyto = $1)
			and type='Note') as x order by x.published desc limit $2`

	if rows, err = config.DB.Query(query, thread, limit); err != nil {
		return util.MakeError(err, "GetThreadFeed")
	}

	defer rows.Close()
	for rows.Next() {
		var Id, Name, Content, AttributedTo, Attachment, MediaType, Preview, Actor, TripCode string
		var Published time.Time
		var Sensitive bool

		err = rows.Scan(&Id, &Name, &Content, &Published, &AttributedTo, &Attachment, &Preview, &Actor, &TripCode, &Sensitive)

		if err != nil {
			return util.MakeError(err, "GetThreadFeed")
		}

		if len(AttributedTo) == 0 {
			AttributedTo = "Anonymous"
		}

		if len(TripCode) > 0 {
			AttributedTo = AttributedTo + " " + TripCode

		}

		if len(Content) > 0 {
			Content = db.ParseCommentCode(Content)
			Content = db.CloseUnclosedTags(Content)
		}

		if len(Preview) > 0 {
			query := `SELECT href FROM activitystream WHERE id = $1 UNION ALL SELECT href FROM cacheactivitystream WHERE id = $1`
			config.DB.QueryRow(query, Preview).Scan(&Preview)
			if len(Preview) > 0 {
				Content = "<img style='float:left;margin:8px' border=0 src='" + Preview + "'>" + Content
			}
		}

		if len(Attachment) > 0 {
			query := `SELECT href, mediatype FROM activitystream WHERE id = $1 UNION ALL SELECT href, mediatype FROM cacheactivitystream WHERE id = $1`
			config.DB.QueryRow(query, Attachment).Scan(&Attachment, &MediaType)
		}

		feedItem := &feeds.Item{
			Id:          Id,                                                                  // Post id
			Title:       Name,                                                                // Subject
			Link:        &feeds.Link{Href: actor.Id + "/" + util.ShortURL(actor.Outbox, Id)}, // "Localized" link to post
			Author:      &feeds.Author{Name: AttributedTo},                                   // Poster name and tripcode
			Description: Content,                                                             // Post comment (and preview)
			Enclosure:   &feeds.Enclosure{Url: Attachment, Type: MediaType},
			Created:     Published, // Post time
		}
		if feedItem.Id != "" && feedItem.Link != nil && feedItem.Author != nil {
			feed.Add(feedItem)
		}
	}
	var feedContent string
	switch feedtype {
	case "atom":
		feedContent, err = feed.ToAtom()
		ctx.Set("Content-Type", "application/atom+xml")
	case "rss":
		feedContent, err = feed.ToRss()
		ctx.Set("Content-Type", "application/rss+xml")
	case "json":
		feedContent, err = feed.ToJSON()
		ctx.Set("Content-Type", "application/json")
	default:
		return ctx.Status(400).SendString("Invalid feed type")
	}

	if err != nil {
		return util.MakeError(err, "GetThreadFeed")
	}

	//TODO: Handle these
	if len(feed.Items) > 0 && feed.Items[0] != nil {
		ctx.Set("Etag", feed.Items[0].Id)
		ctx.Set("Last-Modified", feed.Items[0].Created.UTC().String())
	} else {
		ctx.Set("Etag", "0")
		ctx.Set("Last-Modified", time.Now().UTC().String())
	}
	return ctx.SendString(feedContent)
}

func GetBoardFeed(ctx *fiber.Ctx) error {
	actor, err := activitypub.GetActorFromDB(config.Domain + "/" + ctx.Params("actor"))
	if err != nil || actor.Id == "" {
		return Send404(ctx, "Board /"+ctx.Params("actor")+"/ does not exist")
	}
	feedtype := ctx.Params("feedtype")

	limit, err := strconv.Atoi(ctx.Query("limit"))
	if err != nil {
		limit = 100 // default limit or handle the error appropriately
	}

	if ctx.Query("limit") == "0" {
		limit = 999999999
	}

	now := time.Now()
	feed := &feeds.Feed{
		Title:   "/" + actor.PreferredUsername + "/ - " + actor.Name,
		Link:    &feeds.Link{Href: actor.Id},
		Created: now,
	}

	var rows *sql.Rows
	var query string

	if actor.HasOption(activitypub.OptionReadOnly) {
		query = `select x.id, x.name, x.content, x.published, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (select id, name, content, published, 
		attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor in (select following from following where id in (select id from following where id=$1)) and type='Note' union select id, name, content, published, 
		attributedto, attachment, preview, actor, tripcode, sensitive from cacheactivitystream where actor in (select following from following where id in (select id from follower where id=$1))
		and type='Note') as x order by x.published desc limit $2`
	} else {
		query = `select x.id, x.name, x.content, x.published, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (select id, name, content, published, 
		attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor = $1 and type='Note' union select id, name, content, published, 
		attributedto, attachment, preview, actor, tripcode, sensitive from cacheactivitystream where actor in (select following from following where id in (select id from follower where id=$1))
		and type='Note') as x order by x.published desc limit $2`
	}

	if rows, err = config.DB.Query(query, actor.Id, limit); err != nil {
		return util.MakeError(err, "GetBoardFeed")
	}

	defer rows.Close()
	for rows.Next() {
		var Id, Name, Content, AttributedTo, Attachment, MediaType, Preview, Actor, TripCode string
		var Published time.Time
		var Sensitive bool

		err = rows.Scan(&Id, &Name, &Content, &Published, &AttributedTo, &Attachment, &Preview, &Actor, &TripCode, &Sensitive)

		if err != nil {
			return util.MakeError(err, "GetBoardFeed")
		}

		if len(AttributedTo) == 0 {
			AttributedTo = "Anonymous"
		}

		if len(TripCode) > 0 {
			AttributedTo = AttributedTo + " " + TripCode

		}

		if len(Content) > 0 {
			/*re := regexp.MustCompile(`((\r\n|\r|\n|^)>>(.+)?[^\r\n])`)
			match := re.FindAllStringSubmatch(Content, -1)

			for i, _ := range match {
				Content = strings.Replace(Content, match[i][3], util.ShortURL(actor.Outbox, match[i][0]), 1)
			}*/
			Content = db.ParseCommentCode(Content)
			Content = db.CloseUnclosedTags(Content)
		}

		if len(Preview) > 0 {
			query := `SELECT href FROM activitystream WHERE id = $1 UNION ALL SELECT href FROM cacheactivitystream WHERE id = $1`
			config.DB.QueryRow(query, Preview).Scan(&Preview)
			if len(Preview) > 0 {
				Content = "<img style='float:left;margin:8px' border=0 src='" + Preview + "'>" + Content
			}
		}

		if len(Attachment) > 0 {
			query := `SELECT href, mediatype FROM activitystream WHERE id = $1 UNION ALL SELECT href, mediatype FROM cacheactivitystream WHERE id = $1`
			config.DB.QueryRow(query, Attachment).Scan(&Attachment, &MediaType)
		}

		feedItem := &feeds.Item{
			Id:          Id,                                                                  // Post id
			Title:       Name,                                                                // Subject
			Link:        &feeds.Link{Href: actor.Id + "/" + util.ShortURL(actor.Outbox, Id)}, // "Localized" link to post
			Author:      &feeds.Author{Name: AttributedTo},                                   // Poster name and tripcode
			Description: Content,                                                             // Post comment (and preview)
			Enclosure:   &feeds.Enclosure{Url: Attachment, Type: MediaType},
			Created:     Published, // Post time
		}
		if feedItem.Id != "" && feedItem.Link != nil && feedItem.Author != nil {
			feed.Add(feedItem)
		}
	}

	var feedContent string
	switch feedtype {
	case "atom":
		feedContent, err = feed.ToAtom()
		ctx.Set("Content-Type", "application/atom+xml")
	case "rss":
		feedContent, err = feed.ToRss()
		ctx.Set("Content-Type", "application/rss+xml")
	case "json":
		feedContent, err = feed.ToJSON()
		ctx.Set("Content-Type", "application/json")
	default:
		return ctx.Status(400).SendString("Invalid feed type")
	}

	if err != nil {
		return util.MakeError(err, "GetBoardFeed")
	}

	//TODO: Handle these
	if len(feed.Items) > 0 && feed.Items[0] != nil {
		ctx.Set("Etag", feed.Items[0].Id)
		ctx.Set("Last-Modified", feed.Items[0].Created.UTC().String())
	} else {
		ctx.Set("Etag", "0")
		ctx.Set("Last-Modified", time.Now().UTC().String())
	}
	return ctx.SendString(feedContent)
}
