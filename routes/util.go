package routes

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"html/template"
	"math/rand"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/anomalous69/fchannel/util"

	"github.com/anomalous69/fchannel/activitypub"
	"github.com/anomalous69/fchannel/config"
	"github.com/anomalous69/fchannel/db"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/template/html/v2"
	country "github.com/mikekonan/go-countries"
)

func GetThemeCookie(c *fiber.Ctx) string {
	cookie := c.Cookies("theme")
	if cookie != "" {
		cookies := strings.SplitN(cookie, "=", 2)
		return cookies[0]
	}

	return "default"
}

func GetActorPost(ctx *fiber.Ctx, path string) error {
	obj := activitypub.ObjectBase{Id: config.Domain + path}
	post, err := obj.GetFromPath()

	if err != nil {
		return Send404(ctx, "Post not found", util.MakeError(err, "GetActorPost"))
	}

	enc, err := json.MarshalIndent(post, "", "\t")
	if err != nil {
		return Send500(ctx, "Failed to get post", util.MakeError(err, "GetActorPost"))
	}

	ctx.Response().Header.Set("Content-Type", "application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\"")
	_, err = ctx.Write(enc)
	return util.MakeError(err, "GetActorPost")
}

func ParseOutboxRequest(ctx *fiber.Ctx, actor activitypub.Actor) error {
	pw, _ := util.GetPasswordFromSession(ctx)
	needCaptcha := pw == ""
	contentType := util.GetContentType(ctx.Get("content-type"))

	if contentType == "multipart/form-data" || contentType == "application/x-www-form-urlencoded" {
		hasCaptcha, err := util.BoardHasAuthType(actor.PreferredUsername, "captcha")
		if err != nil {
			return Send500(ctx, "Failed to post", util.MakeError(err, "ParseOutboxRequest"))
		}

		valid, err := db.CheckCaptcha(ctx.FormValue("captcha"))
		if err != nil {
			return Send500(ctx, "Failed to validate captcha", util.MakeError(err, "ParseOutboxRequest"))
		}
		if !needCaptcha || (hasCaptcha && valid) {
			header, _ := ctx.FormFile("file")
			if header != nil {
				f, _ := header.Open()
				defer f.Close()
				if header.Size > (int64(config.MaxAttachmentSize) << 20) {
					return Send400(ctx, "File too large, maximum file size is "+util.ConvertSize(int64(config.MaxAttachmentSize)))
				} else if isBanned, err := db.IsMediaBanned(f); err == nil && isBanned {
					return Send403(ctx, "Attached file is banned")
				} else if err != nil { //TODO: remove this?
					return Send500(ctx, "Failed to post", util.MakeError(err, "ParseOutboxRequest"))
				}

				contentType, _ := util.GetFileContentType(f)
				if actor.Type == "flash" && len(util.EscapeString(ctx.FormValue("inReplyTo"))) == 0 && (contentType != "application/x-shockwave-flash" && contentType != "video/x-flv") {
					return Send400(ctx, "New threads on this board must have a SWF or Flash Video file")
				}

				if !util.SupportedMIMEType(contentType) {
					return Send400(ctx, "File type ("+contentType+") not supported on this board")
				}
			}

			nObj, err := db.ObjectFromForm(ctx, activitypub.CreateObject("Note"))
			if err != nil {
				return Send500(ctx, "Failed to post", util.MakeError(err, "ParseOutboxRequest"))
			}

			op := len(nObj.InReplyTo) - 1
			if op >= 0 {
				if nObj.InReplyTo[op].Id == "" {
					if actor.PreferredUsername == "overboard" {
						return ctx.SendStatus(400)
					}
				}
			}

			if !actor.Restricted {
				nObj.Sensitive = true
			}

			if actor.HasOption(activitypub.OptionAnonymous) {
				nObj.AttributedTo = ""
			}

			if !actor.HasOption(activitypub.OptionTripcode) && nObj.TripCode != "#Admin" && nObj.TripCode != "#Moderator" && nObj.TripCode != "#Janitor" && nObj.TripCode != "##" {
				nObj.TripCode = ""
			}

			if actor.HasOption(activitypub.OptionFlag) {
				nObj.Alias = "cc:" + util.GetCC(ctx.Get("PosterIP"))
			}

			if actor.HasOption(activitypub.OptionID | activitypub.OptionFlag) {
				//TODO: better way to pass IP to
				if ctx.Get("PosterIP") == "172.16.0.1" || util.IsTorExit(ctx.Get("PosterIP")) {
					nObj.Alias = nObj.Alias + "id:HiddenID"
				} else {
					input := []byte(ctx.Get("PosterIP"))
					hasher := sha256.New()
					hasher.Write(input)
					sha := base64.URLEncoding.EncodeToString(hasher.Sum(nil))

					uniqID := string(sha)

					nObj.Alias = nObj.Alias + "id:" + uniqID
				}
			}

			nObj.Actor = config.Domain + "/" + actor.PreferredUsername

			if locked, _ := nObj.InReplyTo[0].IsLocked(); locked {
				return Send403(ctx, "Thread is locked")
			}

			nObj, err = nObj.Write()
			if err != nil {
				return Send500(ctx, "Failed to post", util.MakeError(err, "ParseOutboxRequest"))
			}

			if len(nObj.To) == 0 && actor.PreferredUsername != "overboard" {
				if err := actor.ArchivePosts(); err != nil {
					return Send500(ctx, "Failed to post", util.MakeError(err, "ParseOutboxRequest"))
				}
			}

			go func(nObj activitypub.ObjectBase) {
				activity, err := nObj.CreateActivity("Create")
				if err != nil {
					config.Log.Printf("ParseOutboxRequest Create Activity: %s", err)
				}

				activity, err = activity.AddFollowersTo()
				if err != nil {
					config.Log.Printf("ParseOutboxRequest Add FollowersTo: %s", err)
				}

				if err := activity.MakeRequestInbox(); err != nil {
					config.Log.Printf("ParseOutboxRequest MakeRequestInbox: %s", err)
				}
			}(nObj)

			go func(obj activitypub.ObjectBase) {
				err := obj.SendEmailNotify()

				if err != nil {
					config.Log.Println(err)
				}
			}(nObj)

			var id string
			//op := len(nObj.InReplyTo) - 1
			if op >= 0 {
				if nObj.InReplyTo[op].Id == "" {
					if actor.PreferredUsername == "overboard" {
						return ctx.SendStatus(400)
					}
					id = nObj.Id
				} else {
					id = nObj.InReplyTo[0].Id + "|" + nObj.Id
				}
			}

			if len(ctx.Get("PosterIP")) > 1 || len(ctx.Get("pwd")) > 0 {
				query := `INSERT INTO "identify" (id, ip, password) VALUES ($1, $2, crypt($3, gen_salt('bf')))`
				_, err = config.DB.Exec(query, nObj.Id, ctx.Get("PosterIP"), ctx.FormValue("pwd"))
				if err != nil {
					return Send500(ctx, "Failed to post", util.MakeError(err, "ParseOutboxRequest"))
				}
			}

			ctx.Response().Header.Set("Status", "200")
			_, err = ctx.Write([]byte(id))
			return Send500(ctx, "Failed to post", util.MakeError(err, "ParseOutboxRequest"))
		} else {
			return Send403(ctx, "Incorrect captcha")
		}
	} else { // json request
		activity, err := activitypub.GetActivityFromJson(ctx)
		if err != nil {
			return util.MakeError(err, "ParseOutboxRequest")
		}

		if res, _ := activity.IsLocal(); res {
			if res := activity.Actor.VerifyHeaderSignature(ctx); err == nil && !res {
				ctx.Response().Header.Set("Status", "403")
				_, err = ctx.Write([]byte(""))
				return util.MakeError(err, "ParseOutboxRequest")
			}

			switch activity.Type {
			case "Create":
				ctx.Response().Header.Set("Status", "403")
				_, err = ctx.Write([]byte(""))

			case "Follow":
				validActor := (activity.Object.Actor != "")
				validLocalActor := (activity.Actor.Id == actor.Id)

				var rActivity activitypub.Activity

				if validActor && validLocalActor {
					rActivity = activity.AcceptFollow()
					rActivity, err = rActivity.SetActorFollowing()

					if err != nil {
						return util.MakeError(err, "ParseOutboxRequest")
					}

					if err := activity.MakeRequestInbox(); err != nil {
						return util.MakeError(err, "ParseOutboxRequest")
					}
				}

				actor, _ := activitypub.GetActorFromDB(config.Domain)
				activitypub.FollowingBoards, err = actor.GetFollowing()

				if err != nil {
					return util.MakeError(err, "ParseOutboxRequest")
				}

				activitypub.Boards, err = activitypub.GetBoardCollection()

				if err != nil {
					return util.MakeError(err, "ParseOutboxRequest")
				}

			case "Delete":
				config.Log.Println("This is a delete")
				ctx.Response().Header.Set("Status", "403")
				_, err = ctx.Write([]byte("could not process activity"))

			case "Note":
				ctx.Response().Header.Set("Status", "403")
				_, err = ctx.Write([]byte("could not process activity"))

			case "New":
				name := activity.Object.Alias
				prefname := activity.Object.Name
				summary := activity.Object.Summary
				restricted := activity.Object.Sensitive
				boardtype := activity.Object.MediaType // Didn't want to add new struct field, close enough
				boardoptionmask := 0                   // Ditto
				if len(activity.Object.Option) > 0 {
					var err error
					boardoptionmask, err = strconv.Atoi(activity.Object.Option[0])
					if err != nil {
						boardoptionmask = 0
					}
				}

				actor, err := db.CreateNewBoard(*activitypub.CreateNewActor(prefname, name, summary, config.AuthReq, restricted, boardtype, boardoptionmask))
				if err != nil {
					return util.MakeError(err, "ParseOutboxRequest")
				}

				if actor.Id != "" {
					var board []activitypub.ObjectBase
					var item activitypub.ObjectBase
					var removed bool = false

					item.Id = actor.Id
					for _, e := range activitypub.FollowingBoards {
						if e.Id != item.Id {
							board = append(board, e)
						} else {
							removed = true
						}
					}

					if !removed {
						board = append(board, item)
					}

					activitypub.FollowingBoards = board
					activitypub.Boards, err = activitypub.GetBoardCollection()
					return util.MakeError(err, "ParseOutboxRequest")
				}

				ctx.Response().Header.Set("Status", "403")
				_, err = ctx.Write([]byte(""))

			default:
				ctx.Response().Header.Set("status", "403")
				_, err = ctx.Write([]byte("could not process activity"))
			}
			if err != nil {
				return util.MakeError(err, "ParseOutboxRequest")
			}
		} else if err != nil {
			return util.MakeError(err, "ParseOutboxRequest")
		} else {
			config.Log.Println("is NOT activity")
			ctx.Response().Header.Set("Status", "403")
			_, err = ctx.Write([]byte("could not process activity"))
			return util.MakeError(err, "ParseOutboxRequest")
		}
	}

	return nil
}

func TemplateFunctions(engine *html.Engine) {
	engine.AddFunc("mod", func(i, j int) bool {
		return i%j == 0
	})

	engine.AddFunc("sub", func(i, j int) int {
		return i - j
	})

	engine.AddFunc("add", func(i, j int) int {
		return i + j
	})

	engine.AddFunc("unixtoreadable", func(u int) string {
		return time.Unix(int64(u), 0).Format("Jan 02, 2006")
	})

	engine.AddFunc("timeToDateLong", func(t time.Time) string {
		day := t.Day()
		suffix := "th"
		switch day {
		case 1, 21, 31:
			suffix = "st"
		case 2, 22:
			suffix = "nd"
		case 3, 23:
			suffix = "rd"
		}
		return t.Format("January 2" + suffix + ", 2006 MST")
	})

	engine.AddFunc("timeToDateTimeLong", func(t time.Time) string {
		day := t.Day()
		suffix := "th"
		switch day {
		case 1, 21, 31:
			suffix = "st"
		case 2, 22:
			suffix = "nd"
		case 3, 23:
			suffix = "rd"
		}
		return t.Format("January 2" + suffix + ", 2006 at 15:04 UTC")
	})

	engine.AddFunc("timeToReadableLong", func(t time.Time) string {
		return t.Format("01/02/06(Mon)15:04:05")
	})

	engine.AddFunc("timeToUnix", func(t time.Time) string {
		return fmt.Sprint(t.Unix())
	})

	engine.AddFunc("proxy", util.MediaProxy)

	// previously short
	engine.AddFunc("shortURL", util.ShortURL)

	engine.AddFunc("parseAttachment", db.ParseAttachment)

	engine.AddFunc("parseContent", db.ParseContent)

	engine.AddFunc("formatContent", db.FormatContent)

	engine.AddFunc("shortImg", util.ShortImg)

	engine.AddFunc("convertSize", util.ConvertSize)

	engine.AddFunc("isOnion", util.IsOnion)

	engine.AddFunc("parseReplyLink", func(actorId string, op string, id string, content string) template.HTML {
		actor, _ := activitypub.FingerActor(actorId)
		title := strings.ReplaceAll(db.ParseLinkTitle(actor.Id+"/", op, content), `/\&lt;`, ">")
		link := "<a href=\"/" + actor.PreferredUsername + "/" + util.ShortURL(actor.Outbox, op) + "#" + util.ShortURL(actor.Outbox, id) + "\" title=\"" + title + "\" class=\"replyLink\">&gt;&gt;" + util.ShortURL(actor.Outbox, id) + "</a>"
		return template.HTML(link)
	})

	engine.AddFunc("shortExcerpt", func(post activitypub.ObjectBase) template.HTML {
		var returnString string

		if post.Name != "" {
			returnString = post.Name + "| " + post.Content
		} else {
			returnString = post.Content
		}

		re := regexp.MustCompile(`(^(.|\r\n|\n){100})`)

		match := re.FindStringSubmatch(returnString)

		if len(match) > 0 {
			returnString = match[0] + "..."
		}

		returnString = strings.ReplaceAll(returnString, "<", "&lt;")
		returnString = strings.ReplaceAll(returnString, ">", "&gt;")

		re = regexp.MustCompile(`(^.+\|)`)

		match = re.FindStringSubmatch(returnString)

		if len(match) > 0 {
			returnString = strings.Replace(returnString, match[0], "<b>"+match[0]+"</b>", 1)
			returnString = strings.Replace(returnString, "|", ":", 1)
		}

		return template.HTML(returnString)
	})

	engine.AddFunc("parseLinkTitle", func(board string, op string, content string) string {
		nContent := db.ParseLinkTitle(board, op, content)
		nContent = strings.ReplaceAll(nContent, `/\&lt;`, ">")

		return nContent
	})

	engine.AddFunc("parseLink", func(board activitypub.Actor, link string) string {
		var obj = activitypub.ObjectBase{
			Id: link,
		}

		var OP string
		if OP, _ = obj.GetOP(); OP == obj.Id {
			return board.Name + "/" + util.ShortURL(board.Outbox, obj.Id)
		}

		return board.Name + "/" + util.ShortURL(board.Outbox, OP) + "#" + util.ShortURL(board.Outbox, link)
	})

	engine.AddFunc("showArchive", func(actor activitypub.Actor) bool {
		col, err := actor.GetCollectionTypeLimit("Archive", 1)

		if err != nil || len(col.OrderedItems) == 0 {
			return false
		}

		return true
	})

	engine.AddFunc("parseIDandFlag", func(input string) template.HTML {
		var html string
		re := regexp.MustCompile(`id:\S{8}`)
		id := re.FindString(input)

		re = regexp.MustCompile(`cc:\S{2}`)
		cc := re.FindString(input)

		if id != "" {
			var r, g, b int
			var txtcol, bgcol string
			//var shadcol string
			id = strings.TrimPrefix(id, "id:")
			if id == "HiddenID" {
				bgcol = "rgb(255, 255, 255)"
				txtcol = "#000"
			} else {
				h := md5.New()
				h.Write([]byte(id))
				var seed uint64 = binary.BigEndian.Uint64(h.Sum(nil))
				rand.Seed(int64(seed))
				r = rand.Intn(256)
				g = rand.Intn(256)
				b = rand.Intn(256)
				bgcol = "rgb(" + strconv.Itoa(r) + ", " + strconv.Itoa(g) + ", " + strconv.Itoa(b) + ")"
				var l float64 = ((0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)) / 255)
				if l > 0.5 {
					txtcol = "#000"
				} else {
					txtcol = "#FFF"
				}
			}
			html = " <span class=\"posteruid id_" + id + "\">(ID: <span class=\"id\" style=\"background-color: " + bgcol + "; color: " + txtcol + ";\">" + id + "</span>)</span>"
		}
		if cc != "" {
			var countryname string
			cc = strings.TrimPrefix(cc, "cc:")
			//TODO: remove external library for country
			switch cc {
			case "xp":
				countryname = "Tor/Proxy"
			default:
				if posterCountry, ok := country.ByAlpha2CodeStr(cc); ok {
					countryname = posterCountry.Name().String()
				} else {
					countryname = "Unknown/Hidden"
				}
			}
			html = html + " <span title=\"" + countryname + "\" class=\"flag flag-" + cc + "\"></span>"
		}
		return template.HTML(html)
	})

	engine.AddFunc("parseEmail", func(input string) template.HTML {
		var html string
		if len(input) > 1 {
			email := regexp.MustCompile(`email:.+@.+\..+`)
			if email.MatchString(input) {
				addr := strings.TrimPrefix(input, "email:")
				html += "<a href='mailto:" + addr + "' class='userEmail'>"
			}
		}
		return template.HTML(html)
	})

	engine.AddFunc("timeUntil", func(to time.Time, from ...time.Time) string {
		var duration time.Duration
		if len(from) > 0 {
			duration = to.Sub(from[0].UTC())
		} else {
			duration = to.Sub(time.Now().UTC())
		}
		years := int(duration.Hours() / 24 / 365)
		months := int(duration.Hours()/24/30) % 12
		days := int(duration.Hours()/24) % 30
		hours := int(duration.Hours()) % 24
		minutes := int(duration.Minutes()) % 60
		seconds := int(duration.Seconds()) % 60

		var timeStrings []string
		if years > 1 {
			timeStrings = append(timeStrings, fmt.Sprintf("%d years", years))
		} else if years == 1 {
			timeStrings = append(timeStrings, fmt.Sprintf("%d year", years))
		}
		if months > 1 {
			timeStrings = append(timeStrings, fmt.Sprintf("%d months", months))
		} else if months == 1 {
			timeStrings = append(timeStrings, fmt.Sprintf("%d month", months))
		}
		if days > 1 {
			timeStrings = append(timeStrings, fmt.Sprintf("%d days", days))
		} else if days == 1 {
			timeStrings = append(timeStrings, fmt.Sprintf("%d day", days))
		}
		if hours > 1 {
			timeStrings = append(timeStrings, fmt.Sprintf("%d hours", hours))
		} else if hours == 1 {
			timeStrings = append(timeStrings, fmt.Sprintf("%d hour", hours))
		}
		if minutes > 1 {
			timeStrings = append(timeStrings, fmt.Sprintf("%d minutes", minutes))
		} else if minutes == 1 {
			timeStrings = append(timeStrings, fmt.Sprintf("%d minute", minutes))
		}
		if years == 0 && months == 0 && days == 0 && minutes == 0 {
			if seconds == 1 {
				timeStrings = append(timeStrings, fmt.Sprintf("%d second", seconds))
			} else if seconds > 1 {
				timeStrings = append(timeStrings, fmt.Sprintf("%d seconds", seconds))
			}
		}

		if len(timeStrings) == 0 {
			return "0 seconds"
		}

		if len(timeStrings) == 1 {
			return timeStrings[0]
		}

		last := timeStrings[len(timeStrings)-1]
		timeStrings = timeStrings[:len(timeStrings)-1]
		return strings.Join(timeStrings, ", ") + " and " + last
	})

	engine.AddFunc("maxFileSize", func() string {
		return util.ConvertSize(int64(config.MaxAttachmentSize))
	})

	engine.AddFunc("boardtypeFromInReplyTo", func(id string) string {
		//TODO: Hangs entire instance if remote instance is down
		// so for now always fallback to "image" if remote
		if !strings.Contains(id, config.Domain) {
			return "image"
		}
		re := regexp.MustCompile(`.+\/`)
		actorid := strings.TrimSuffix(re.FindString(id), "/")
		actor, err := activitypub.GetActor(actorid)
		if err != nil {
			return "image"
		}
		return actor.BoardType
	})

	engine.AddFunc("tegakiSupportsImage", func(contentType string) bool {
		switch contentType {
		case "image/png", "image/jpeg":
			return true
		default:
			return false
		}
	})

	// Add HasBoardOption for templates
	engine.AddFunc("HasBoardOption", func(actor interface{}, option int) bool {
		if actor == nil {
			return false
		}
		// Accept both *activitypub.Actor and activitypub.Actor
		switch a := actor.(type) {
		case *activitypub.Actor:
			return a != nil && a.HasOption(option)
		case activitypub.Actor:
			return (&a).HasOption(option)
		default:
			return false
		}
	})
}

func StatusTemplate(num int) func(ctx *fiber.Ctx, msg string, err ...error) error {
	return func(ctx *fiber.Ctx, msg string, err ...error) error {
		var m string
		if len(msg) > 0 {
			m = msg
		} else {
			switch num {
			case 400:
				m = "Your request could not be processed due to errors."
			case 403:
				m = "You are not allowed to access this resource."
			case 404:
				m = "The resource you are trying to access does not exist."
			case 500:
				m = "The server encountered an error and could not complete your request."
			}
		}

		var data PageData
		var statusData StatusData

		data.Boards = activitypub.Boards
		data.Themes = &config.Themes
		data.ThemeCookie = GetThemeCookie(ctx)
		data.Referer = ctx.Get("referer")

		statusData.Code = num
		statusData.Meaning = http.StatusText(num)
		statusData.Message = m

		// Display error on page if instance admin
		_, modcred := util.GetPasswordFromSession(ctx)
		if hasAuth, modlevel := util.HasAuth(modcred, config.Domain); (hasAuth) && (modlevel == "admin") && (err != nil) {
			statusData.Error = err
		}

		data.Title = strconv.Itoa(num) + " " + statusData.Meaning

		return ctx.Status(num).Render("status", fiber.Map{
			"page":   data,
			"status": statusData,
		}, "layouts/main")
	}
}

var Send400 = StatusTemplate(400)
var Send401 = StatusTemplate(401)
var Send403 = StatusTemplate(403)
var Send404 = StatusTemplate(404)
var Send500 = StatusTemplate(500)
