package routes

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/anomalous69/fchannel/activitypub"
	"github.com/anomalous69/fchannel/config"
	"github.com/anomalous69/fchannel/db"
	"github.com/anomalous69/fchannel/util"
	"github.com/gofiber/fiber/v2"
)

func ActorInbox(ctx *fiber.Ctx) error {
	actor, _ := activitypub.GetActorFromDB(config.Domain + "/" + ctx.Params("actor"))
	if actor.HasOption(activitypub.OptionReadOnly) {
		return ctx.SendStatus(404)
	}

	activity, err := activitypub.GetActivityFromJson(ctx)

	if err != nil {
		return util.MakeError(err, "ActorInbox")
	}
	if activity.Actor == nil || activity.Actor.PublicKey == nil || activity.Actor.PublicKey.Id == "" {
		nActor, err := activitypub.FingerActor("")
		if activity.Actor != nil {
			if activity.Actor.Id != "" {
				nActor, err = activitypub.FingerActor(activity.Actor.Id)
			}
		}
		if err != nil {
			return util.MakeError(err, "ActorInbox")
		}

		activity.Actor = &nActor
	}
	if activity.Actor == nil || activity.Actor.PublicKey == nil || activity.Actor.PublicKey.Id == "" {
		return util.MakeError(errors.New("missing actor or public key for signature verification"), "ActorInbox")
	}

	if !activity.Actor.VerifyHeaderSignature(ctx) {
		response := activity.Reject()
		return response.MakeRequestInbox()
	}

	switch activity.Type {
	case "Create":
		for _, e := range activity.To {
			actor := activitypub.Actor{Id: e}
			if err := actor.ProcessInboxCreate(activity); err != nil {
				return util.MakeError(err, "ActorInbox")
			}

			if err := actor.SendToFollowers(activity); err != nil {
				return util.MakeError(err, "ActorInbox")
			}
		}

		for _, e := range activity.Cc {
			actor := activitypub.Actor{Id: e}
			if err := actor.ProcessInboxCreate(activity); err != nil {
				return util.MakeError(err, "ActorInbox")
			}
		}

	case "Delete":
		for _, e := range activity.To {
			actor, err := activitypub.GetActorFromDB(e)
			if err != nil {
				continue // try again
				// return util.MakeError(err, "ActorInbox")
			}

			if actor.Id != "" && actor.Id != config.Domain {
				if activity.Object.Replies != nil {
					for _, k := range activity.Object.Replies.OrderedItems {
						if err := k.Tombstone(); err != nil {
							return util.MakeError(err, "ActorInbox")
						}
					}
				}

				if err := activity.Object.Tombstone(); err != nil {
					return util.MakeError(err, "ActorInbox")
				}

				if err := actor.UnArchiveLast(); err != nil {
					return util.MakeError(err, "ActorInbox")
				}
				break
			}
		}

	case "Follow":
		for _, e := range activity.To {
			if _, err := activitypub.GetActorFromDB(e); err == nil {
				response := activity.AcceptFollow()
				response, err := response.SetActorFollower()

				if err != nil {
					return util.MakeError(err, "ActorInbox")
				}

				if err := response.MakeRequestInbox(); err != nil {
					return util.MakeError(err, "ActorInbox")
				}

				alreadyFollowing, err := response.Actor.IsAlreadyFollowing(response.Object.Id)

				if err != nil {
					return util.MakeError(err, "ActorInbox")
				}

				objActor, err := activitypub.FingerActor(response.Object.Actor)

				if err != nil || objActor.Id == "" {
					return util.MakeError(err, "ActorInbox")
				}

				reqActivity := activitypub.Activity{Id: objActor.Following}
				remoteActorFollowingCol, err := reqActivity.GetCollection()

				if err != nil {
					return util.MakeError(err, "ActorInbox")
				}

				alreadyFollow := false

				for _, e := range remoteActorFollowingCol.Items {
					if e.Id == response.Actor.Id {
						alreadyFollowing = true
					}
				}

				autoSub, err := response.Actor.GetAutoSubscribe()

				if err != nil {
					return util.MakeError(err, "ActorInbox")
				}

				if autoSub && !alreadyFollow && alreadyFollowing {
					followActivity, err := response.Actor.MakeFollowActivity(response.Object.Actor)

					if err != nil {
						return util.MakeError(err, "ActorInbox")
					}

					if err := followActivity.MakeRequestOutbox(); err != nil {
						return util.MakeError(err, "ActorInbox")
					}
				}
			} else if err != nil {
				return util.MakeError(err, "ActorInbox")
			} else {
				config.Log.Println("follow request for rejected")
				response := activity.Reject()
				return response.MakeRequestInbox()
			}
		}

	case "Reject":
		if activity.Object.Object.Type == "Follow" {
			config.Log.Println("follow rejected")
			if _, err := activity.SetActorFollowing(); err != nil {
				return util.MakeError(err, "ActorInbox")
			}
		}
	}

	return nil
}

func PostActorOutbox(ctx *fiber.Ctx) error {
	//var activity activitypub.Activity
	actor, err := activitypub.GetActorFromPath(ctx.Path(), "/")
	if err != nil {
		return util.MakeError(err, "ActorOutbox")
	}

	if activitypub.AcceptActivity(ctx.Get("Accept")) {
		return actor.GetOutbox(ctx)
	}

	return ParseOutboxRequest(ctx, actor)
}

func ActorFollowing(ctx *fiber.Ctx) error {
	actor, _ := activitypub.GetActorFromDB(config.Domain + "/" + ctx.Params("actor"))
	return actor.GetFollowingResp(ctx)
}

func ActorFollowers(ctx *fiber.Ctx) error {
	actor, _ := activitypub.GetActorFromDB(config.Domain + "/" + ctx.Params("actor"))
	return actor.GetFollowersResp(ctx)
}

func MakeActorPost(ctx *fiber.Ctx) error {
	pw, _ := util.GetPasswordFromSession(ctx)
	// Empty captcha
	if len(pw) == 0 && len(ctx.FormValue("captcha")) == 0 {
		return Send401(ctx, "Missing captcha")
	}

	var ban db.Ban
	//TODO: Bad and ugly
	ban.IP, _, _, _, _ = db.IsIPBanned(ctx.IP())
	if len(ban.IP) > 1 {
		return ctx.Redirect(ctx.BaseURL()+"/banned", 301)
	}

	header, _ := ctx.FormFile("file")

	// Missing attachment on non-textboard
	actor, _ := activitypub.GetActorByNameFromDB(ctx.FormValue("boardName"))

	if len(ctx.FormValue("inReplyTo")) == 0 && actor.HasOption(activitypub.OptionReadOnly) {
		return Send403(ctx, "Cannot make new thread, board is read-only")
	}

	if len(ctx.FormValue("inReplyTo")) == 0 && header == nil && actor.BoardType != "text" {
		return Send400(ctx, "File is required for new threads")
	}

	if actor.BoardType == "text" {
		// Textboard: Tried to post with attachment
		if header != nil {
			return Send400(ctx, "Posting files is disabled on this board")
		}
		// Textboard: New thread, empty subject
		if len(ctx.FormValue("inReplyTo")) == 0 && len(ctx.FormValue("subject")) == 0 {
			return Send400(ctx, "Subject is required for new textboard thread")
		}
		// Textboard: Empty comment
		if len(ctx.FormValue("comment")) == 0 {
			return Send400(ctx, "Comment is required for textboard reply")
		}
	}

	// Missing both file and comment
	if header == nil && len(ctx.FormValue("comment")) == 0 {
		return Send400(ctx, "Comment or File is required for new posts")
	}

	// Trying to reply to non-existant thread
	//TODO: Handle DB error
	if ctx.FormValue("inReplyTo") != "" && !db.IsValidThread(ctx.FormValue("inReplyTo")) {
		return Send400(ctx, "\""+ctx.FormValue("inReplyTo")+"\" is not a valid thread on this server")
	}

	var file multipart.File

	if header != nil {
		file, _ = header.Open()
		contentType, _ := util.GetFileContentType(file)
		// Only allow new threads on flash type boards with SWF or FLV files
		if actor.BoardType == "flash" && len(util.EscapeString(ctx.FormValue("inReplyTo"))) == 0 && contentType != "application/x-shockwave-flash" && contentType != "video/x-flv" {
			file.Close()
			return Send400(ctx, "New threads on this board must have a SWF or Flash Video file")
		}
	}

	// Attachment filename too long
	if file != nil && len(header.Filename) > 256 {
		return Send400(ctx, "Filename too long, maximum length is 256 characters")
	}

	// Attachment filesize larger than config max size
	if file != nil && header.Size > (int64(config.MaxAttachmentSize)<<20) {
		return Send400(ctx, "File too large, maximum file size is "+util.ConvertSize(int64(config.MaxAttachmentSize)))
	}

	// Redirect to instance index when post matches blacklist regex
	if is, _, regex := util.IsPostBlacklist(ctx.FormValue("comment")); is {
		config.Log.Println("Blacklist post blocked \nRegex: " + regex + "\n" + ctx.FormValue("comment"))
		return ctx.Redirect(ctx.BaseURL()+"/", 301)
	}

	// New thread empty subject or comment
	if ctx.FormValue("inReplyTo") == "" && strings.TrimSpace(ctx.FormValue("comment")) == "" && ctx.FormValue("subject") == "" {
		return Send400(ctx, "Subject or Comment is required for new threads")
	}

	// Comment text too long
	if len(ctx.FormValue("comment")) > 4500 {
		return Send400(ctx, "Comment is longer than 4500 characters")
	}

	// Too many newlines
	if strings.Count(ctx.FormValue("comment"), "\r\n") > 50 || strings.Count(ctx.FormValue("comment"), "\n") > 50 || strings.Count(ctx.FormValue("comment"), "\r") > 50 {
		return Send400(ctx, "Comment contains too many newlines")
	}

	// Name, Subject, or Options are too long
	if len(ctx.FormValue("subject")) > 100 || len(ctx.FormValue("name")) > 100 || len(ctx.FormValue("options")) > 100 {
		return Send400(ctx, "Name, Subject, or Options field(s) contain more than 100 characters")
	}

	b := bytes.Buffer{}
	we := multipart.NewWriter(&b)

	if file != nil {
		var fw io.Writer

		fw, err := we.CreateFormFile("file", header.Filename)

		if err != nil {
			return util.MakeError(err, "ActorPost")
		}
		_, err = io.Copy(fw, file)

		if err != nil {
			return util.MakeError(err, "ActorPost")
		}
	}

	reply, _ := db.ParseCommentForReply(ctx.FormValue("comment"))

	form, _ := ctx.MultipartForm()

	if form.Value == nil {
		return util.MakeError(errors.New("form value nil"), "ActorPost")
	}

	for key, r0 := range form.Value {
		if key == "captcha" {
			err := we.WriteField(key, ctx.FormValue("captchaCode")+":"+ctx.FormValue("captcha"))
			if err != nil {
				return util.MakeError(err, "ActorPost")
			}
		} else if key == "name" {
			name, tripcode, _ := util.CreateNameTripCode(ctx)

			err := we.WriteField(key, name)
			if err != nil {
				return util.MakeError(err, "ActorPost")
			}

			err = we.WriteField("tripcode", tripcode)
			if err != nil {
				return util.MakeError(err, "ActorPost")
			}
		} else {
			err := we.WriteField(key, r0[0])
			if err != nil {
				return util.MakeError(err, "ActorPost")
			}
		}
	}

	if (ctx.FormValue("inReplyTo") == "" || ctx.FormValue("inReplyTo") == "NaN") && reply != "" {
		err := we.WriteField("inReplyTo", reply)
		if err != nil {
			return util.MakeError(err, "ActorPost")
		}
	}

	we.Close()

	sendTo := ctx.FormValue("sendTo")

	if len(ctx.FormValue("inReplyTo")) > 0 {
		re := regexp.MustCompile(`.+\/`)
		replyactorid := strings.TrimSuffix(re.FindString(ctx.FormValue("inReplyTo")), "/")
		replyactor, err := activitypub.GetActor(replyactorid)
		// Reject replies with files when the OP is from a textboard
		if replyactor.BoardType == "text" && header != nil {
			return Send400(ctx, "The thread you are replying to is from a text-only board, attachments are not allowed")
		}
		if err == nil {
			local, _ := replyactor.IsLocal()
			if local {
				sendTo = replyactor.Outbox
			}
		} else {
			query := `select id from following where following = $1 AND following IN (
    SELECT id FROM actor WHERE (optionsmask & $2) = 0
) LIMIT 1;`
			if err := config.DB.QueryRow(query, replyactorid, activitypub.OptionReadOnly).Scan(&replyactorid); err == nil {
				if actor, err := activitypub.GetActor(replyactorid); err == nil {
					sendTo = actor.Outbox
				}
			}
		}
	}
	//actorid := strings.TrimSuffix(re.FindString(ctx.FormValue("inReplyTo")), "/")
	//sendTo = actorid + "/outbox"
	//actor, _ := activitypub.GetActorFromPath(actorid, "/")
	//sendTo = actor.Outbox
	//}

	req, err := http.NewRequest("POST", sendTo, &b)

	if err != nil {
		return util.MakeError(err, "ActorPost")
	}

	req.Header.Set("Content-Type", we.FormDataContentType())
	if c := ctx.Cookies("session_token"); c != "" {
		// This is a hack to pass through the token while we still make
		// requests to the outbox
		req.Header.Set("Authorization", "Bearer "+c)
	}
	req.Header.Set("PosterIP", ctx.IP())

	resp, err := util.RouteProxy(req)
	if err != nil {
		return util.MakeError(err, "ActorPost")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Send500(ctx, "Failed to post", util.MakeError(err, "ActorPost"))
	}

	if resp.StatusCode == 200 {
		var postid string
		var obj activitypub.ObjectBase

		obj = db.ParseOptions(ctx, obj)
		re := regexp.MustCompile(`\S*\|`)
		postid = re.ReplaceAllString(string(body), "${1}")
		if len(postid) > 0 {
			ctx.Set("postid", postid)
		}
		for _, e := range obj.Option {
			if e == "noko" || e == "nokosage" {
				return ctx.Redirect(ctx.BaseURL()+"/"+ctx.FormValue("boardName")+"/"+util.ShortURL(ctx.FormValue("sendTo"), string(body)), 301)
			}
		}

		if ctx.FormValue("returnTo") == "catalog" {
			return ctx.Redirect(ctx.BaseURL()+"/"+ctx.FormValue("boardName")+"/catalog", 301)
		} else {
			return ctx.Redirect(ctx.BaseURL()+"/"+ctx.FormValue("boardName"), 301)
		}
	}

	if resp.StatusCode == 403 {
		//TODO: Better way to do this?
		ctx.Response().Header.Set("Status", "403")
		ctx.Set(fiber.HeaderContentType, fiber.MIMETextHTML)
		return ctx.Send(body)
	}

	return ctx.Redirect(ctx.BaseURL()+"/"+ctx.FormValue("boardName"), 301)
}

func ActorPost(ctx *fiber.Ctx) error {
	actor, err := activitypub.GetActorByNameFromDB(ctx.Params("actor"))

	if err != nil {
		return nil
	}

	// this is a activitpub json request return json instead of html page
	if activitypub.AcceptActivity(ctx.Get("Accept")) {
		GetActorPost(ctx, ctx.Path())
		return nil
	}

	re := regexp.MustCompile(`\w+$`)
	postId := re.FindString(ctx.Path())

	inReplyTo, _ := db.GetPostIDFromNum(postId)

	// check if actually OP if not redirect to op to get full thread
	var obj = activitypub.ObjectBase{Id: inReplyTo}
	if OP, _ := obj.GetOP(); OP != obj.Id {
		return ctx.Redirect(ctx.BaseURL()+"/"+actor.PreferredUsername+"/"+util.ShortURL(actor.Outbox, OP)+"#"+util.ShortURL(actor.Outbox, inReplyTo), http.StatusMovedPermanently)
	}

	collection, err := obj.GetCollectionFromPath()

	if err != nil {
		return Send404(ctx, "Thread not found")
	}

	var data PageData

	if collection.Actor.Id != "" {
		data.Board.Post.Actor = collection.Actor.Id
		data.Board.InReplyTo = inReplyTo
	}

	if len(collection.OrderedItems) > 0 {
		data.Posts = append(data.Posts, collection.OrderedItems[0])
	}

	data.Board.Name = actor.Name
	data.Board.PrefName = actor.PreferredUsername
	data.Board.To = actor.Outbox
	data.Board.Actor = actor
	data.Board.Summary = actor.Summary
	data.Board.ModCred, _ = util.GetPasswordFromSession(ctx)
	data.Board.Domain = config.Domain
	data.Board.Restricted = actor.Restricted
	data.Board.BoardType = actor.BoardType
	data.ReturnTo = "feed"
	data.PostType = "reply"

	if len(data.Posts) > 0 {
		data.PostId = util.ShortURL(data.Board.To, data.Posts[0].Id)
	}

	// Ignore captcha if we're authenticated
	if data.Board.ModCred != data.Board.Domain && data.Board.ModCred != data.Board.Actor.Id {
		capt, err := util.GetRandomCaptcha()
		if err != nil {
			return Send500(ctx, "Failed to get random captcha", util.MakeError(err, "ActorPost"))
		}
		data.Board.Captcha = config.Domain + "/" + capt
		data.Board.CaptchaCode = db.GetCaptchaCode(data.Board.Captcha)
	}

	data.Instance, err = activitypub.GetActorFromDB(config.Domain)
	if err != nil {
		return util.MakeError(err, "PostGet")
	}

	data.Key = config.Key
	data.Boards = activitypub.Boards

	data.Title = "/" + data.Board.PrefName + "/ - " + data.PostId

	if len(data.Posts) > 0 {
		data.Meta.Description = data.Posts[0].Content
		data.Meta.Url = data.Posts[0].Id
		data.Meta.Title = data.Posts[0].Name
		if data.Posts[0].Preview != nil {
			data.Meta.Preview = data.Posts[0].Preview.Href
		}
	}

	data.Themes = &config.Themes
	data.ThemeCookie = GetThemeCookie(ctx)

	data.ServerVersion = config.Version

	return ctx.Render("npost", fiber.Map{
		"page": data,
	}, "layouts/main")
}

func ActorCatalog(ctx *fiber.Ctx) error {
	actorName := ctx.Params("actor")
	actor, err := activitypub.GetActorByNameFromDB(actorName)

	if err != nil {
		return util.MakeError(err, "ActorCatalog")
	}

	collection, err := actor.GetCatalogCollection()

	if err != nil {
		return util.MakeError(err, "ActorCatalog")
	}

	var data PageData
	data.Board.Name = actor.Name
	data.Board.PrefName = actor.PreferredUsername
	data.Board.InReplyTo = ""
	data.Board.To = actor.Outbox
	data.Board.Actor = actor
	data.Board.Summary = actor.Summary
	data.Board.ModCred, _ = util.GetPasswordFromSession(ctx)
	data.Board.Domain = config.Domain
	data.Board.Restricted = actor.Restricted
	data.Board.BoardType = actor.BoardType
	data.Key = config.Key
	data.ReturnTo = "catalog"
	data.PostType = "new"

	data.Board.Post.Actor = actor.Id

	data.Instance, err = activitypub.GetActorFromDB(config.Domain)
	if err != nil {
		return util.MakeError(err, "CatalogGet")
	}

	// Ignore captcha if we're authenticated
	if data.Board.ModCred != data.Board.Domain && data.Board.ModCred != data.Board.Actor.Id {
		capt, err := util.GetRandomCaptcha()
		if err != nil {
			return Send500(ctx, "Failed to get random captcha", util.MakeError(err, "ActorCatalog"))
		}
		data.Board.Captcha = config.Domain + "/" + capt
		data.Board.CaptchaCode = db.GetCaptchaCode(data.Board.Captcha)
	}

	data.Title = "/" + data.Board.PrefName + "/ - Catalog"

	data.Boards = activitypub.Boards
	data.Posts = collection.OrderedItems

	data.Meta.Description = data.Board.Summary
	data.Meta.Url = data.Board.Actor.Id
	data.Meta.Title = data.Title

	data.Themes = &config.Themes
	data.ThemeCookie = GetThemeCookie(ctx)

	data.ServerVersion = config.Version

	return ctx.Render("catalog", fiber.Map{
		"page": data,
	}, "layouts/main")
}

func ActorPosts(ctx *fiber.Ctx) error {
	actor, err := activitypub.GetActorByNameFromDB(ctx.Params("actor"))

	if err != nil {
		return Send404(ctx, "Board /"+ctx.Params("actor")+"/ not found")
	}

	if activitypub.AcceptActivity(ctx.Get("Accept")) {
		actor.GetInfoResp(ctx)
		return nil
	}

	var page int
	if postNum := ctx.Query("page"); postNum != "" {
		if page, err = strconv.Atoi(postNum); err != nil {
			return util.MakeError(err, "OutboxGet")
		}
	}

	collection, err := actor.WantToServePage(page)
	if err != nil {
		return util.MakeError(err, "OutboxGet")
	}

	var offset = 15
	var pages []int
	pageLimit := (float64(collection.TotalItems) / float64(offset))

	if pageLimit > 11 && !actor.HasOption(activitypub.OptionReadOnly) {
		pageLimit = 11
	}

	for i := 0.0; i < pageLimit; i++ {
		pages = append(pages, int(i))
	}

	var data PageData
	data.Board.Name = actor.Name
	data.Board.PrefName = actor.PreferredUsername
	data.Board.Summary = actor.Summary
	data.Board.InReplyTo = ""
	data.Board.To = actor.Outbox
	data.Board.Actor = actor
	data.Board.ModCred, _ = util.GetPasswordFromSession(ctx)
	data.Board.Domain = config.Domain
	data.Board.Restricted = actor.Restricted
	data.Board.BoardType = actor.BoardType
	data.CurrentPage = page
	data.ReturnTo = "feed"
	data.PostType = "new"

	data.Board.Post.Actor = actor.Id

	// Ignore captcha if we're authenticated
	if data.Board.ModCred != data.Board.Domain && data.Board.ModCred != data.Board.Actor.Id {
		capt, err := util.GetRandomCaptcha()
		if err != nil {
			return Send500(ctx, "Failed to get random captcha", util.MakeError(err, "ActorPosts"))
		}
		data.Board.Captcha = config.Domain + "/" + capt
		data.Board.CaptchaCode = db.GetCaptchaCode(data.Board.Captcha)
	}

	data.Title = "/" + actor.PreferredUsername + "/ - " + actor.Name

	data.Key = config.Key

	data.Boards = activitypub.Boards
	data.Posts = collection.OrderedItems

	data.Pages = pages
	data.TotalPage = len(data.Pages) - 1

	data.Meta.Description = data.Board.Summary
	data.Meta.Url = data.Board.Actor.Id
	data.Meta.Title = data.Title

	data.Themes = &config.Themes
	data.ThemeCookie = GetThemeCookie(ctx)

	data.ServerVersion = config.Version

	//TODO: merge flash and list into index-list.html, make ActorList server index-list.html
	return ctx.Render("index-"+data.Board.BoardType, fiber.Map{
		"page": data,
	}, "layouts/main")
}

func ActorArchive(ctx *fiber.Ctx) error {
	actorName := ctx.Params("actor")
	actor, err := activitypub.GetActorByNameFromDB(actorName)

	if err != nil {
		return util.MakeError(err, "ActorArchive")
	}

	collection, err := actor.GetCollectionType("Archive")

	if err != nil {
		return util.MakeError(err, "ActorArchive")
	}

	var returnData PageData
	returnData.Board.Name = actor.Name
	returnData.Board.PrefName = actor.PreferredUsername
	returnData.Board.InReplyTo = ""
	returnData.Board.To = actor.Outbox
	returnData.Board.Actor = actor
	returnData.Board.Summary = actor.Summary
	returnData.Board.ModCred, _ = util.GetPasswordFromSession(ctx)
	returnData.Board.Domain = config.Domain
	returnData.Board.Restricted = actor.Restricted
	returnData.Board.BoardType = actor.BoardType
	returnData.Key = config.Key
	returnData.ReturnTo = "archive"

	returnData.Board.Post.Actor = actor.Id

	returnData.Instance, err = activitypub.GetActorFromDB(config.Domain)
	if err != nil {
		Send500(ctx, "Failed to get archive", err)
	}

	//TODO: Check if ActorArchive needs captcha
	capt, err := util.GetRandomCaptcha()
	if err != nil {
		return util.MakeError(err, "ActorArchive")
	}
	returnData.Board.Captcha = config.Domain + "/" + capt
	returnData.Board.CaptchaCode = db.GetCaptchaCode(returnData.Board.Captcha)

	returnData.Title = "/" + actor.PreferredUsername + "/ - Archive"

	returnData.Boards = activitypub.Boards

	returnData.Posts = collection.OrderedItems

	returnData.Meta.Description = returnData.Board.Summary
	returnData.Meta.Url = returnData.Board.Actor.Id
	returnData.Meta.Title = returnData.Title

	returnData.Themes = &config.Themes
	returnData.ThemeCookie = GetThemeCookie(ctx)

	returnData.ServerVersion = config.Version

	return ctx.Render("archive", fiber.Map{
		"page": returnData,
	}, "layouts/main")
}

func ActorList(ctx *fiber.Ctx) error {
	actor, err := activitypub.GetActorByNameFromDB(ctx.Params("actor"))

	if err != nil {
		return Send404(ctx, "Board /"+ctx.Params("actor")+"/ not found")
	}

	collection, err := actor.GetCollectionType("Note")
	if err != nil {
		return util.MakeError(err, "OutboxGet")
	}

	var data PageData
	data.Board.Name = actor.Name
	data.Board.PrefName = actor.PreferredUsername
	data.Board.InReplyTo = ""
	data.Board.To = actor.Outbox
	data.Board.Actor = actor
	data.Board.ModCred, _ = util.GetPasswordFromSession(ctx)
	data.Board.Domain = config.Domain
	data.Board.Restricted = actor.Restricted
	data.Board.BoardType = actor.BoardType
	data.ReturnTo = "list"
	data.PostType = "new"

	data.Board.Post.Actor = actor.Id

	// Ignore captcha if we're authenticated
	if data.Board.ModCred != data.Board.Domain && data.Board.ModCred != data.Board.Actor.Id {
		capt, err := util.GetRandomCaptcha()
		if err != nil {
			return Send500(ctx, "Failed to get random captcha", util.MakeError(err, "ActorList"))
		}
		data.Board.Captcha = config.Domain + "/" + capt
		data.Board.CaptchaCode = db.GetCaptchaCode(data.Board.Captcha)
	}

	data.Title = "/" + actor.PreferredUsername + "/ - Thread list"
	data.Key = config.Key

	data.Boards = activitypub.Boards
	data.Posts = collection.OrderedItems

	data.Meta.Description = data.Board.Summary
	data.Meta.Url = data.Board.Actor.Id
	data.Meta.Title = data.Title

	data.Themes = &config.Themes
	data.ThemeCookie = GetThemeCookie(ctx)

	data.ServerVersion = config.Version

	return ctx.Render("list", fiber.Map{
		"page": data,
	}, "layouts/main")
}

func GetActorOutbox(ctx *fiber.Ctx) error {
	actor, _ := activitypub.GetActorFromPath(ctx.Path(), "/")

	collection, _ := actor.GetCollection()
	collection.AtContext.Context = "https://www.w3.org/ns/activitystreams"
	collection.Actor = &actor

	collection.TotalItems, _ = actor.GetPostTotal()
	collection.TotalImgs, _ = actor.GetImgTotal()

	enc, _ := json.Marshal(collection)

	ctx.Response().Header.Add("Content-Type", config.ActivityStreams)
	_, err := ctx.Write(enc)

	return err
}
