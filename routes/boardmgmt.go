package routes

import (
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"net/smtp"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/anomalous69/fchannel/activitypub"
	"github.com/anomalous69/fchannel/config"

	"github.com/corona10/goimagehash"

	"github.com/anomalous69/fchannel/db"
	"github.com/anomalous69/fchannel/util"
	"github.com/gofiber/fiber/v2"
)

func BoardBanMedia(ctx *fiber.Ctx) error {
	var err error

	postID := ctx.Query("id")
	board := ctx.Query("board")

	_, auth := util.GetPasswordFromSession(ctx)

	if postID == "" || auth == "" {
		return Send401(ctx, "You are not authenticated")
	}

	var col activitypub.Collection
	activity := activitypub.Activity{Id: postID}

	if col, err = activity.GetCollection(); err != nil {
		return Send400(ctx, "Post does not exist or server encountered issue with database", util.MakeError(err, "BoardBanMedia"))
	}

	if len(col.OrderedItems) == 0 {
		return Send400(ctx, "Could not ban media, post not found")
	}

	if len(col.OrderedItems[0].Attachment) == 0 {
		return Send400(ctx, "Could not ban media, post has no attachment")
	}

	var actor activitypub.Actor
	actor.Id = col.OrderedItems[0].Actor

	if has, _ := util.HasAuth(auth, actor.Id); !has {
		return Send403(ctx, "You are not authorized to ban media on board /"+ctx.Query("board")+"/")
	}

	re := regexp.MustCompile(config.Domain)
	file := re.ReplaceAllString(col.OrderedItems[0].Attachment[0].Href, "")

	f, err := os.Open("." + file)

	if err != nil {
		return Send500(ctx, "Failed to ban media (file does not exist or server is unable to read)", util.MakeError(err, "BoardBanMedia"))
	}

	defer f.Close()

	//TODO: Fall back to old method if anything fails
	mimetype, _ := util.GetFileContentType(f)
	config.Log.Println("mimetype: " + mimetype)
	if mimetype == "image/jpeg" || mimetype == "image/png" || mimetype == "image/gif" {
		image, _, err := image.Decode(f)
		if err != nil {
			return Send500(ctx, "Failed to ban media (server failed to decode image)", util.MakeError(err, "BoardBanMedia"))
		}

		phash, err := goimagehash.PerceptionHash(image)
		if err != nil {
			return Send500(ctx, "Failed to ban media (server failed to hash image)", util.MakeError(err, "BoardBanMedia"))
		}

		config.Log.Println("Banning hash: ", uint64(phash.GetHash()))

		query := `insert into bannedimages (phash) values ($1)`
		if _, err := config.DB.Exec(query, uint64(phash.GetHash())); err != nil {
			return Send500(ctx, "Failed to ban media (server failed to insert into database)", util.MakeError(err, "BoardBanMedia"))
		}
	} else {
		bytes := make([]byte, 2048)

		if _, err = f.Read(bytes); err != nil {
			return Send500(ctx, "Failed to ban media (server failed to read file)", util.MakeError(err, "BoardBanMedia"))
		}

		if banned, err := db.IsMediaBanned(f); err == nil && !banned {
			query := `insert into bannedmedia (hash) values ($1)`
			if _, err := config.DB.Exec(query, util.HashBytes(bytes)); err != nil {
				return Send500(ctx, "Failed to ban media", util.MakeError(err, "BoardBanMedia"))
			}
		}
	}

	var isOP bool
	var local bool
	var obj activitypub.ObjectBase
	obj.Id = postID
	obj.Actor = actor.Id

	if isOP, _ = obj.CheckIfOP(); !isOP {
		if err := obj.Tombstone(); err != nil {
			return Send500(ctx, "Failed to ban media", util.MakeError(err, "BoardBanMedia"))
		}
	} else {
		if err := obj.TombstoneReplies(); err != nil {
			return Send500(ctx, "Failed to ban media", util.MakeError(err, "BoardBanMedia"))
		}
	}

	if local, _ = obj.IsLocal(); local {
		if err := obj.DeleteRequest(); err != nil {
			return Send500(ctx, "Failed to ban media", util.MakeError(err, "BoardBanMedia"))
		}
	}

	if err := actor.UnArchiveLast(); err != nil {
		return Send500(ctx, "Failed to ban media", util.MakeError(err, "BoardBanMedia"))
	}

	var OP string
	if len(col.OrderedItems[0].InReplyTo) > 0 {
		OP = col.OrderedItems[0].InReplyTo[0].Id
	}

	if !isOP {
		if !local && OP != "" {
			return ctx.Redirect("/"+board+"/"+util.RemoteShort(OP), http.StatusSeeOther)
		} else if OP != "" {
			return ctx.Redirect(OP, http.StatusSeeOther)
		}
	}

	return ctx.Redirect("/"+board, http.StatusSeeOther)
}

func BoardDelete(ctx *fiber.Ctx) error {
	var err error

	postID := ctx.Query("id")
	board := ctx.Query("board")

	_, auth := util.GetPasswordFromSession(ctx)

	if postID == "" || auth == "" {
		return Send401(ctx, "You are not authenticated")
	}

	var col activitypub.Collection
	activity := activitypub.Activity{Id: postID}

	if col, err = activity.GetCollection(); err != nil {
		return Send400(ctx, "Post does not exist or server encountered issue with database", util.MakeError(err, "BoardDelete"))
	}

	var OP string
	var actor activitypub.Actor

	if len(col.OrderedItems) == 0 {
		actor, err = activitypub.GetActorByNameFromDB(board)

		if err != nil {
			return Send400(ctx, "Board does not exist or server encountered issue with database", util.MakeError(err, "BoardDelete"))
		}
	} else {
		if len(col.OrderedItems[0].InReplyTo) > 0 {
			OP = col.OrderedItems[0].InReplyTo[0].Id
		} else {
			OP = postID
		}

		actor.Id = col.OrderedItems[0].Actor
	}

	if has, _ := util.HasAuth(auth, actor.Id); !has {
		return Send403(ctx, "You are not authorized to ban media on board /"+ctx.Query("board")+"/")
	}

	var isOP bool
	obj := activitypub.ObjectBase{Id: postID}

	if isOP, _ = obj.CheckIfOP(); !isOP {
		if err := obj.Tombstone(); err != nil {
			return Send500(ctx, "Failed to ban poster", util.MakeError(err, "BoardDelete"))
		}
	} else {
		if err := obj.TombstoneReplies(); err != nil {
			return Send500(ctx, "Failed to ban poster", util.MakeError(err, "BoardDelete"))
		}
	}

	var local bool

	if local, _ = obj.IsLocal(); local {
		if err := obj.DeleteRequest(); err != nil {
			return Send500(ctx, "Failed to ban poster", util.MakeError(err, "BoardDelete"))
		}
	}

	if err := actor.UnArchiveLast(); err != nil {
		return Send500(ctx, "Failed to ban poster", util.MakeError(err, "BoardDelete"))
	}

	if ctx.Query("manage") == "t" {
		return ctx.Redirect("/"+config.Key+"/"+board, http.StatusSeeOther)
	}

	if !isOP {
		if !local && OP != "" {
			return ctx.Redirect("/"+board+"/"+util.RemoteShort(OP), http.StatusSeeOther)
		} else if OP != "" {
			return ctx.Redirect(OP, http.StatusSeeOther)
		}
	}

	return ctx.Redirect("/"+board, http.StatusSeeOther)
}

func BoardDeleteAttach(ctx *fiber.Ctx) error {
	var err error

	postID := ctx.Query("id")
	board := ctx.Query("board")

	_, auth := util.GetPasswordFromSession(ctx)

	if postID == "" || auth == "" {
		return Send401(ctx, "You are not authenticated")
	}

	var col activitypub.Collection
	activity := activitypub.Activity{Id: postID}

	if col, err = activity.GetCollection(); err != nil {
		return Send400(ctx, "Post does not exist or server encountered issue with database", util.MakeError(err, "BoardDeleteAttach"))
	}

	var OP string
	var actor activitypub.Actor

	if len(col.OrderedItems) == 0 {
		actor, err = activitypub.GetActorByNameFromDB(board)

		if err != nil {
			return Send400(ctx, "Board does not exist or server encountered issue with database", util.MakeError(err, "BoardDeleteAttach"))
		}
	} else {
		if len(col.OrderedItems[0].InReplyTo) > 0 {
			OP = col.OrderedItems[0].InReplyTo[0].Id
		} else {
			OP = postID
		}

		actor.Id = col.OrderedItems[0].Actor
	}

	if has, _ := util.HasAuth(auth, actor.Id); !has {
		return Send403(ctx, "You are not authorized to delete media on board /"+ctx.Query("board")+"/")
	}

	obj := activitypub.ObjectBase{Id: postID}

	if err := obj.DeleteAttachmentFromFile(); err != nil {
		return Send500(ctx, "Failed to delete attachment", util.MakeError(err, "BoardDeleteAttach"))
	}

	if err := obj.TombstoneAttachment(); err != nil {
		return Send500(ctx, "Failed to delete attachment", util.MakeError(err, "BoardDeleteAttach"))
	}

	if err := obj.DeletePreviewFromFile(); err != nil {
		return Send500(ctx, "Failed to delete attachment", util.MakeError(err, "BoardDeleteAttach"))
	}

	if err := obj.TombstonePreview(); err != nil {
		return Send500(ctx, "Failed to delete attachment", util.MakeError(err, "BoardDeleteAttach"))
	}

	if ctx.Query("manage") == "t" {
		return ctx.Redirect("/"+config.Key+"/"+board, http.StatusSeeOther)
	} else if local, _ := obj.IsLocal(); !local && OP != "" {
		return ctx.Redirect("/"+board+"/"+util.RemoteShort(OP), http.StatusSeeOther)
	} else if OP != "" {
		return ctx.Redirect(OP, http.StatusSeeOther)
	}

	return ctx.Redirect("/"+board, http.StatusSeeOther)
}

func BoardMarkSensitive(ctx *fiber.Ctx) error {
	var err error

	postID := ctx.Query("id")
	board := ctx.Query("board")

	_, auth := util.GetPasswordFromSession(ctx)

	if postID == "" || auth == "" {
		return Send401(ctx, "You are not authenticated")
	}

	var col activitypub.Collection
	activity := activitypub.Activity{Id: postID}

	if col, err = activity.GetCollection(); err != nil {
		return Send400(ctx, "Post does not exist or server encountered issue with database", util.MakeError(err, "BoardMarkSensitive"))
	}

	var OP string
	var actor activitypub.Actor

	if len(col.OrderedItems) == 0 {
		actor, err = activitypub.GetActorByNameFromDB(board)

		if err != nil {
			return Send400(ctx, "Board does not exist or server encountered issue with database", util.MakeError(err, "BoardMarkSensitive"))
		}
	} else {
		if len(col.OrderedItems[0].InReplyTo) > 0 {
			OP = col.OrderedItems[0].InReplyTo[0].Id
		} else {
			OP = postID
		}

		actor.Id = col.OrderedItems[0].Actor
	}

	if has, _ := util.HasAuth(auth, actor.Id); !has {
		return Send403(ctx, "You are not authorized to ban media on board /"+ctx.Query("board")+"/")
	}

	obj := activitypub.ObjectBase{Id: postID}

	if err = obj.MarkSensitive(true); err != nil {
		return Send500(ctx, "Failed to mark post as sensitive", util.MakeError(err, "BoardMarkSensitive"))
	}

	if isOP, _ := obj.CheckIfOP(); !isOP && OP != "" {
		if local, _ := obj.IsLocal(); !local {
			return ctx.Redirect("/"+board+"/"+util.RemoteShort(OP), http.StatusSeeOther)
		}

		return ctx.Redirect(OP, http.StatusSeeOther)
	}

	return ctx.Redirect("/"+board, http.StatusSeeOther)
}

// TODO routes/BoardRemove
func BoardRemove(ctx *fiber.Ctx) error {
	return ctx.SendString("board remove")
}

// TODO routes/BoardAddToIndex
func BoardAddToIndex(ctx *fiber.Ctx) error {
	return ctx.SendString("board add to index")
}

func BoardPopArchive(ctx *fiber.Ctx) error {
	actor, err := activitypub.GetActorFromDB(config.Domain)

	if err != nil {
		return util.MakeError(err, "BoardPopArchive")
	}

	if has := actor.HasValidation(ctx); !has {
		return Send403(ctx, "You are not authorized to pop archives")
	}

	id := ctx.Query("id")
	board := ctx.Query("board")

	var obj = activitypub.ObjectBase{Id: id}

	if err := obj.SetRepliesType("Note"); err != nil {
		return Send500(ctx, "Failed to pop archive", util.MakeError(err, "BoardPopArchive"))
	}

	return ctx.Redirect("/"+board+"/archive", http.StatusSeeOther)
}

func BoardAutoSubscribe(ctx *fiber.Ctx) error {
	actor, err := activitypub.GetActorFromDB(config.Domain)

	if err != nil {
		return util.MakeError(err, "BoardAutoSubscribe")
	}

	if has := actor.HasValidation(ctx); !has {
		return util.MakeError(err, "BoardAutoSubscribe")
	}

	board := ctx.Query("board")

	if actor, err = activitypub.GetActorByNameFromDB(board); err != nil {
		return util.MakeError(err, "BoardAutoSubscribe")
	}

	if err := actor.SetAutoSubscribe(); err != nil {
		return util.MakeError(err, "BoardAutoSubscribe")
	}

	if autoSub, _ := actor.GetAutoSubscribe(); autoSub {
		if err := actor.AutoFollow(); err != nil {
			return util.MakeError(err, "BoardAutoSubscribe")
		}
	}

	return ctx.Redirect("/"+config.Key+"/"+board, http.StatusSeeOther)
}

func BoardBlacklist(ctx *fiber.Ctx) error {
	actor, err := activitypub.GetActorFromDB(config.Domain)

	if err != nil {
		return Send400(ctx, "Board does not exist or server encountered issue with database", util.MakeError(err, "BoardBlacklist"))
	}

	if has := actor.HasValidation(ctx); !has {
		return Send403(ctx, "You are not authorized to modify regex blacklist")
	}

	if ctx.Method() == "GET" {
		if id := ctx.Query("remove"); id != "" {
			i, _ := strconv.Atoi(id)
			if err := util.DeleteRegexBlacklist(i); err != nil {
				return Send400(ctx, "Failed to add regex to blacklist", util.MakeError(err, "BoardBlacklist"))
			}
		}
	} else {
		regex := ctx.FormValue("regex")
		testCase := ctx.FormValue("testCase")

		if regex == "" {
			return ctx.Redirect("/", http.StatusSeeOther)
		}

		re := regexp.MustCompile(regex)

		if testCase == "" {
			if err := util.WriteRegexBlacklist(regex); err != nil {
				return util.MakeError(err, "BoardBlacklist")
			}
		} else if re.MatchString(testCase) {
			if err := util.WriteRegexBlacklist(regex); err != nil {
				return util.MakeError(err, "BoardBlacklist")
			}
		}
	}

	return ctx.Redirect("/"+config.Key+"#regex", http.StatusSeeOther)
}

func ReportPost(ctx *fiber.Ctx) error {
	id := ctx.FormValue("id")
	board := ctx.FormValue("board")
	reason := ctx.FormValue("comment")
	close := ctx.FormValue("close")
	referer := ctx.BaseURL() + "/" + board
	if strings.Contains(ctx.FormValue("referer"), ctx.BaseURL()+"/"+board) && !strings.Contains(ctx.FormValue("referer"), "make-report") {
		referer = ctx.FormValue("referer")
	}

	actor, err := activitypub.GetActorByNameFromDB(board)

	if err != nil {
		return util.MakeError(err, "BoardReport")
	}

	var ban db.Ban
	//TODO: Bad and ugly
	ban.IP, _, _, _, _ = db.IsIPBanned(ctx.IP())
	if len(ban.IP) > 1 {
		return ctx.Redirect(ctx.BaseURL()+"/banned", 301)
	}

	_, auth := util.GetPasswordFromSession(ctx)

	var obj = activitypub.ObjectBase{Id: id}

	if close == "1" { //TODO: Check this, HasAuth returns string which is put in "err"
		if auth, err := util.HasAuth(auth, actor.Id); !auth {
			config.Log.Println(err)
			return Send404(ctx, "") //TODO: FILL IN
		}

		if local, _ := obj.IsLocal(); !local {
			if err := db.CloseLocalReport(obj.Id, board); err != nil {
				config.Log.Println(err)
				return Send404(ctx, "", err) //TODO: FILL IN
			}

			return ctx.Redirect("/"+config.Key+"/"+board, http.StatusSeeOther)
		}

		if err := obj.DeleteReported(); err != nil {
			config.Log.Println(err)
			return Send404(ctx, "", err) //TODO: FILL IN
		}

		return ctx.Redirect("/"+config.Key+"/"+board, http.StatusSeeOther)
	}

	if local, _ := obj.IsLocal(); !local {
		if err := db.CreateLocalReport(id, board, reason); err != nil {
			config.Log.Println(err)
			return Send404(ctx, "", err) //TODO: FILL IN
		}

		return ctx.Redirect("/"+board+"/"+util.RemoteShort(obj.Id), http.StatusSeeOther)
	}

	var captcha = ctx.FormValue("captchaCode") + ":" + ctx.FormValue("captcha")

	if len(reason) > 100 {
		return Send400(ctx, "Report comment limit is 100 characters")
	}

	if len(strings.TrimSpace(reason)) == 0 {
		return Send400(ctx, "No report reason was provided")
	}

	if ok, _ := db.CheckCaptcha(captcha); !ok && close != "1" {
		return Send403(ctx, "Invalid captcha")
	}

	if err := db.CreateLocalReport(obj.Id, board, reason); err != nil {
		config.Log.Println(err)
		return Send404(ctx, "") //TODO: FILL IN
	}

	if len(config.NtfyURL) > 0 {
		req, _ := http.NewRequest("POST", config.NtfyURL,
			strings.NewReader(id+"\nReason: "+reason))
		req.Header.Set("Click", "ntfy://"+config.NtfyURL) // Opens ntfy app, also prevents message copy to clipboard on tap.
		req.Header.Set("Actions", "view, View post, "+id+", clear=true")
		if len(config.NtfyAuth) > 0 {
			req.Header.Set("Authorization", config.NtfyAuth)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			config.Log.Printf("Error sending report to ntfy server: %s", err)
		}
		resp.Body.Close()
	}

	if setup := util.IsEmailSetup(); setup {
		from := config.SiteEmail
		user := config.SiteEmailUsername
		pass := config.SiteEmailPassword
		to := config.SiteEmailNotifyTo
		body := fmt.Sprintf("New report: %s\nReason: %s", id, reason)

		msg := "From: Fchan <" + from + ">\n" +
			"To: " + to + "\n" +
			"Subject: IB Report\n\n" +
			body

		err := smtp.SendMail(config.SiteEmailServer+":"+config.SiteEmailPort,
			smtp.PlainAuth(from, user, pass, config.SiteEmailServer),
			from, []string{to}, []byte(msg))

		if err != nil {
			config.Log.Printf("Error when sending report email: %s", err)
		}
	}

	// TEMP FIX WHILE WAITING FOR NEW FORK
	return ctx.Redirect(referer, http.StatusSeeOther)

}

func ReportGet(ctx *fiber.Ctx) error {
	actor, _ := activitypub.GetActor(ctx.Query("actor"))
	var ban db.Ban
	//TODO: Bad and ugly
	ban.IP, _, _, _, _ = db.IsIPBanned(ctx.IP())
	if len(ban.IP) > 1 {
		return ctx.Redirect(ctx.BaseURL()+"/banned", 301)
	}

	var data PageData
	data.Board.Actor = actor
	data.Board.Name = actor.Name
	data.Board.PrefName = actor.PreferredUsername
	data.Board.Summary = actor.Summary
	data.Board.InReplyTo = ctx.Query("post")
	data.Board.To = actor.Outbox
	data.Board.Restricted = actor.Restricted
	data.Board.BoardType = actor.BoardType

	capt, err := util.GetRandomCaptcha()

	if err != nil {
		return util.MakeError(err, "OutboxGet")
	}

	data.Board.Captcha = config.Domain + "/" + capt
	data.Board.CaptchaCode = db.GetCaptchaCode(data.Board.Captcha)

	data.Meta.Description = data.Board.Summary
	data.Meta.Url = data.Board.Actor.Id
	data.Meta.Title = data.Title

	data.Instance, err = activitypub.GetActorFromDB(config.Domain)

	data.Themes = &config.Themes
	data.ThemeCookie = GetThemeCookie(ctx)

	data.ServerVersion = config.Version

	data.Key = config.Key
	data.Board.ModCred, _ = util.GetPasswordFromSession(ctx)
	data.Board.Domain = config.Domain
	data.Boards = activitypub.Boards

	data.Referer = config.Domain + "/" + actor.PreferredUsername
	if strings.Contains(ctx.Get("referer"), config.Domain+"/"+actor.PreferredUsername) && !strings.Contains(ctx.Get("referer"), "make-report") {
		data.Referer = ctx.Get("referer")
	}

	return ctx.Render("report", fiber.Map{"page": data}, "layouts/main")
}

func Sticky(ctx *fiber.Ctx) error {
	id := ctx.Query("id")
	board := ctx.Query("board")

	actor, _ := activitypub.GetActorByNameFromDB(board)

	_, auth := util.GetPasswordFromSession(ctx)

	if id == "" || auth == "" {
		return util.MakeError(errors.New("no auth"), "Sticky")
	}

	var obj = activitypub.ObjectBase{Id: id}
	col, _ := obj.GetCollectionFromPath()

	if len(col.OrderedItems) < 1 {
		if has, _ := util.HasAuth(auth, actor.Id); !has {
			return util.MakeError(errors.New("no auth"), "Sticky")
		}

		obj.MarkSticky(actor.Id)

		return ctx.Redirect("/"+board, http.StatusSeeOther)
	}

	actor.Id = col.OrderedItems[0].Actor

	var OP string
	if len(col.OrderedItems[0].InReplyTo) > 0 && col.OrderedItems[0].InReplyTo[0].Id != "" {
		OP = col.OrderedItems[0].InReplyTo[0].Id
	} else {
		OP = id
	}

	if has, _ := util.HasAuth(auth, actor.Id); !has {
		return util.MakeError(errors.New("no auth"), "Sticky")
	}

	obj.MarkSticky(actor.Id)

	var op = activitypub.ObjectBase{Id: OP}
	if local, _ := op.IsLocal(); !local {
		return ctx.Redirect("/"+board+"/"+util.RemoteShort(OP), http.StatusSeeOther)
	} else {
		return ctx.Redirect(OP, http.StatusSeeOther)
	}
}

func Lock(ctx *fiber.Ctx) error {
	id := ctx.Query("id")
	board := ctx.Query("board")

	actor, _ := activitypub.GetActorByNameFromDB(board)

	_, auth := util.GetPasswordFromSession(ctx)

	if id == "" || auth == "" {
		return util.MakeError(errors.New("no auth"), "Lock")
	}

	var obj = activitypub.ObjectBase{Id: id}
	col, _ := obj.GetCollectionFromPath()

	if len(col.OrderedItems) < 1 {
		if has, _ := util.HasAuth(auth, actor.Id); !has {
			return util.MakeError(errors.New("no auth"), "Lock")
		}

		obj.MarkLocked(actor.Id)

		return ctx.Redirect("/"+board, http.StatusSeeOther)
	}

	actor.Id = col.OrderedItems[0].Actor

	var OP string
	if len(col.OrderedItems[0].InReplyTo) > 0 && col.OrderedItems[0].InReplyTo[0].Id != "" {
		OP = col.OrderedItems[0].InReplyTo[0].Id
	} else {
		OP = id
	}

	if has, _ := util.HasAuth(auth, actor.Id); !has {
		return util.MakeError(errors.New("no auth"), "Lock")
	}

	obj.MarkLocked(actor.Id)

	var op = activitypub.ObjectBase{Id: OP}
	if local, _ := op.IsLocal(); !local {
		return ctx.Redirect("/"+board+"/"+util.RemoteShort(OP), http.StatusSeeOther)
	} else {
		return ctx.Redirect(OP, http.StatusSeeOther)
	}
}

func BanGet(ctx *fiber.Ctx) error {
	actor, _ := activitypub.GetActor(ctx.Query("actor"))
	post := ctx.Query("post")

	_, auth := util.GetPasswordFromSession(ctx)

	if auth == "" {
		return util.MakeError(errors.New("no auth"), "Ban")
	}

	if has, _ := util.HasAuth(auth, actor.Id); !has {
		return util.MakeError(errors.New("no auth"), "Ban")
	}

	ip := db.GetPostIP(post)
	if len(ip) == 0 {
		return util.MakeError(errors.New("Post ID \""+ctx.Query("post")+"\" has no IP address"), "Ban")
	}

	// TODO: Check if IP is already permanently banned
	// TODO: Handle permanent bans better, maybe consider anything > 100 years to be permanent rather than checking for single date (9999-12-31 00:00:00)
	// TODO: Display post content (name, comment, image (make this blurred with click through))
	// TODO: More information like other IP's banned in this range
	// TODO: Range bans + IPv6 prefix support

	var data PageData
	data.Board.Actor = actor
	data.Board.Name = actor.Name
	data.Board.PrefName = actor.PreferredUsername
	data.Board.Summary = actor.Summary
	data.Board.InReplyTo = post
	data.Board.To = actor.Outbox
	data.Board.Restricted = actor.Restricted
	data.Board.BoardType = actor.BoardType

	data.Meta.Description = data.Board.Summary
	data.Meta.Url = data.Board.Actor.Id
	data.Meta.Title = data.Title

	data.Instance, _ = activitypub.GetActorFromDB(config.Domain)

	data.Themes = &config.Themes
	data.ThemeCookie = GetThemeCookie(ctx)

	data.ServerVersion = config.Version

	data.Key = config.Key
	data.Board.ModCred, _ = util.GetPasswordFromSession(ctx)
	data.Board.Domain = config.Domain
	data.Boards = activitypub.Boards

	var baninfo BanInfo
	baninfo.Bans, _ = db.GetAllBansForIP(ip)

	data.Referer = config.Domain + "/" + actor.PreferredUsername
	if strings.Contains(ctx.Get("referer"), config.Domain+"/"+actor.PreferredUsername) && !strings.Contains(ctx.Get("referer"), "ban") {
		data.Referer = ctx.Get("referer")
	}

	return ctx.Render("ban", fiber.Map{"page": data, "baninfo": baninfo}, "layouts/main")
}

func BanPost(ctx *fiber.Ctx) error {
	id := ctx.FormValue("id")
	board := ctx.FormValue("board")

	actor, _ := activitypub.GetActorByNameFromDB(board)

	_, auth := util.GetPasswordFromSession(ctx)

	if id == "" || auth == "" {
		return util.MakeError(errors.New("no auth"), "Ban")
	}

	if has, _ := util.HasAuth(auth, actor.Id); !has {
		return util.MakeError(errors.New("no auth"), "Ban")
	}

	if len(db.GetPostIP(id)) == 0 {
		return util.MakeError(errors.New("Post ID \""+ctx.Query("post")+"\" has no IP address"), "Ban")
	}

	reason := ctx.FormValue("comment")
	var expires time.Time
	var err error

	expiresStr := ctx.FormValue("expires")
	config.Log.Println(ctx.FormValue("custom-date"))
	if len(ctx.FormValue("custom-date")) > 0 {
		expires, err = time.Parse("2006-01-02T15:04:05.000Z", ctx.FormValue("custom-date"))
		if err != nil {
			return util.MakeError(err, "BanPost")
		}
	} else {
		switch expiresStr {
		case "1day":
			expires = time.Now().AddDate(0, 0, 1)
		case "3days":
			expires = time.Now().AddDate(0, 0, 3)
		case "1week":
			expires = time.Now().AddDate(0, 0, 7)
		case "2weeks":
			expires = time.Now().AddDate(0, 0, 14)
		case "1month":
			//expires = time.Now().AddDate(0, 1, 0)
			expires = time.Now().AddDate(0, 0, 30)
		case "permanent":
			expires = time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)
		default:
			return util.MakeError(errors.New("invalid ban length"), "BanPost")
		}
	}

	expires = expires.UTC()

	query := `INSERT INTO "bannedips" (ip, reason, date, expires) VALUES ((SELECT ip from identify WHERE id=$1 AND ip !='172.16.0.1'), $2, $3, $4);`
	_, err = config.DB.Exec(query, id, reason, time.Now().UTC(), expires)
	if err != nil {
		return util.MakeError(err, "BanPost")
	}

	// Take a little shortcut :)
	if ctx.FormValue("banmedia") == "on" {
		return ctx.Redirect("/banmedia?id=" + id + "&board=" + board)
	} else {
		return ctx.Redirect("/"+board, http.StatusSeeOther)
	}
}
