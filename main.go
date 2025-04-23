package main

import (
	"math/rand"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/anomalous69/fchannel/activitypub"
	"github.com/anomalous69/fchannel/config"
	"github.com/anomalous69/fchannel/db"
	"github.com/anomalous69/fchannel/routes"
	"github.com/anomalous69/fchannel/util"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/encryptcookie"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/template/html/v2"
)

func main() {

	Init()
	defer db.Close()

	// Make should set the version in config.Version based on git describe when building.
	// If FChannel was built without using the Makefile then this won't be set, so first we define
	// the current software version as a fallback, then try to create version string from git describe at runtime.
	// The way version is set could be much better.
	if len(config.Version) == 0 {
		config.Version = "v0.2.0" // REMEMBER TO ALSO UPDATE THE VERSION HERE WHEN ADDING A NEW GIT TAG (format: vMAJOR.MINOR.PATCH)
		stdout, err := exec.Command("git", "describe", "--tags", "--dirty=-dev").Output()
		ver := strings.TrimSpace(string(stdout))
		if err == nil && len(ver) > 0 {
			re := regexp.MustCompile("[0-9]*-g")
			config.Version = re.ReplaceAllString(ver, "")
		}
	}

	// Routing and templates
	template := html.New("./views", ".html")

	routes.TemplateFunctions(template)

	app := fiber.New(fiber.Config{
		AppName:      "FChannel (" + config.Version + ")",
		Views:        template,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
		ServerHeader: "FChannel/" + config.InstanceName,
		ProxyHeader:  config.ProxyHeader,
		BodyLimit:    config.MaxAttachmentSize + 594, // max attachment size + some extra for overhead/headers etc
	})

	app.Use(logger.New())

	cookieKey, err := util.GetCookieKey()

	if err != nil {
		config.Log.Println(err)
	}

	app.Use(encryptcookie.New(encryptcookie.Config{
		Key:    cookieKey,
		Except: []string{"csrf_", "theme"},
	}))

	app.Static("/static", "./views")
	app.Static("/public", "./public")

	// Main actor
	app.Get("/", routes.Index)
	app.Post("/inbox", routes.Inbox)
	app.Post("/outbox", routes.Outbox)
	app.Get("/following", routes.Following)
	app.Get("/followers", routes.Followers)
	app.Get("/feed.:feedtype", routes.GetBoardFeed)

	// Admin routes
	app.All("/"+config.Key+"/", routes.AdminIndex)
	app.Post("/"+config.Key+"/verify", routes.AdminVerify)
	app.All("/"+config.Key+"/follow", routes.AdminFollow)
	app.Post("/"+config.Key+"/addboard", routes.AdminAddBoard)
	app.Post("/"+config.Key+"/newspost", routes.NewsPost)
	app.Get("/"+config.Key+"/newsdelete/:ts", routes.NewsDelete)
	app.Post("/"+config.Key+"/:actor/addjanny", routes.AdminAddJanny)
	app.Post("/"+config.Key+"/:actor/editsummary", routes.AdminEditSummary)
	app.Post("/"+config.Key+"/:actor/setboardtype", routes.AdminSetBoardType)
	app.Get("/"+config.Key+"/:actor/deletejanny", routes.AdminDeleteJanny)
	app.All("/"+config.Key+"/:actor/follow", routes.AdminFollow)
	app.Get("/"+config.Key+"/:actor", routes.AdminActorIndex)

	app.Get("/banned", routes.BannedGet)

	// News routes
	app.Get("/news/:ts", routes.NewsGet)
	app.Get("/news.:feedtype", routes.GetNewsFeed)
	app.Get("/news", routes.NewsGetAll)

	// Board managment
	app.Get("/ban", routes.BanGet)
	app.Post("/ban", routes.BanPost)
	app.Get("/banmedia", routes.BoardBanMedia)
	app.Get("/delete", routes.BoardDelete)
	app.Get("/deleteattach", routes.BoardDeleteAttach)
	app.Get("/marksensitive", routes.BoardMarkSensitive)
	app.Get("/addtoindex", routes.BoardAddToIndex)
	app.Get("/poparchive", routes.BoardPopArchive)
	app.Get("/autosubscribe", routes.BoardAutoSubscribe)
	app.All("/blacklist", routes.BoardBlacklist)
	app.All("/report", routes.ReportPost)
	app.Get("/make-report", routes.ReportGet)
	app.Get("/sticky", routes.Sticky)
	app.Get("/lock", routes.Lock)

	app.Post("/multidelete", routes.MultiDelete)

	// Webfinger routes
	app.Get("/.well-known/webfinger", routes.Webfinger)

	// NodeInfo routes
	app.Get("/.well-known/nodeinfo", routes.NodeInfoDiscover)
	app.Get("/nodeinfo/:version", routes.NodeInfo)

	// API routes
	app.Get("/api/media", routes.Media)

	// Board actor routes
	app.Post("/post", routes.MakeActorPost)
	app.Get("/:actor/catalog", routes.ActorCatalog)
	app.Post("/:actor/inbox", routes.ActorInbox)
	app.Get("/:actor/outbox", routes.GetActorOutbox)
	app.Post("/:actor/outbox", routes.PostActorOutbox)
	app.Get("/:actor/following", routes.ActorFollowing)
	app.Get("/:actor/followers", routes.ActorFollowers)
	app.Get("/:actor/archive", routes.ActorArchive)
	app.Get("/:actor/list", routes.ActorList)
	app.Get("/:actor/feed.:feedtype", routes.GetBoardFeed)
	app.Get("/:actor", routes.ActorPosts)
	app.Get("/:actor/:post", routes.ActorPost)
	app.Get("/:actor/:post/feed.:feedtype", routes.GetThreadFeed)

	// Settings Routes
	app.Post("/settheme", routes.SetTheme)

	db.PrintAdminAuth()

	app.Listen(config.Port)
}

func Init() {
	var actor activitypub.Actor
	var err error

	rand.Seed(time.Now().UnixNano())

	if err = util.CreatedNeededDirectories(); err != nil {
		config.Log.Println(err)
	}

	if err = db.Connect(); err != nil {
		config.Log.Println(err)
	}

	if err = db.EnsureMigrationState(); err != nil {
		config.Log.Println(err)
	}
	if err = db.RunMigrations(); err != nil {
		config.Log.Println(err)
	}

	if err = db.InitInstance(); err != nil {
		config.Log.Println(err)
	}

	if actor, err = activitypub.GetActorFromDB(config.Domain); err != nil {
		config.Log.Println(err)
	}

	if activitypub.FollowingBoards, err = actor.GetFollowing(); err != nil {
		config.Log.Println(err)
	}

	if activitypub.Boards, err = activitypub.GetBoardCollection(); err != nil {
		config.Log.Println(err)
	}

	if config.Key == "" {
		if config.Key, err = util.CreateKey(32); err != nil {
			config.Log.Println(err)
		}
	}

	if err = util.LoadThemes(); err != nil {
		config.Log.Println(err)
	}

	go activitypub.StartupArchive()

	go util.MakeCaptchas(100)

	go db.CheckInactive()
}
