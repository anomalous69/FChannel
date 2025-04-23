package db

import (
	"database/sql"
	"fmt"
	"html/template"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/anomalous69/fchannel/activitypub"
	"github.com/anomalous69/fchannel/config"
	"github.com/anomalous69/fchannel/util"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

type NewsItem struct {
	Title   string
	Content template.HTML
	Time    int
}

type Ban struct {
	IP      string
	Reason  string
	Date    time.Time
	Expires time.Time
}

func Connect() error {
	host := config.DBHost
	port := config.DBPort
	user := config.DBUser
	password := config.DBPassword
	dbname := config.DBName

	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s password=%s "+
		"dbname=%s sslmode=disable", host, port, user, password, dbname)

	_db, err := sql.Open("pgx", psqlInfo)

	if err != nil {
		return util.MakeError(err, "Connect")
	}

	if err := _db.Ping(); err != nil {
		return util.MakeError(err, "Connect")
	}

	config.Log.Println("Successfully connected DB")

	config.DB = _db

	return nil
}

func Close() error {
	err := config.DB.Close()

	return util.MakeError(err, "Close")
}

func RunMigrations() error {
	if config.DB == nil {
		return fmt.Errorf("DB connection is nil")
	}

	driver, err := postgres.WithInstance(config.DB, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("migrate driver: %w", err)
	}

	m, err := migrate.NewWithDatabaseInstance(
		"file://db/migrations",
		"postgres", driver,
	)
	if err != nil {
		return fmt.Errorf("migrate instance: %w", err)
	}

	err = m.Up()
	if err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}

func CreateNewBoard(actor activitypub.Actor) (activitypub.Actor, error) {
	if _, err := activitypub.GetActorFromDB(actor.Id); err == nil {
		return activitypub.Actor{}, util.MakeError(err, "CreateNewBoardDB")
	} else {
		if actor.BoardType != "text" && actor.BoardType != "flash" {
			actor.BoardType = "image"
		}
		query := `insert into actor (type, id, name, preferredusername, inbox, outbox, following, followers, summary, restricted, boardtype) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`
		_, err := config.DB.Exec(query, actor.Type, actor.Id, actor.Name, actor.PreferredUsername, actor.Inbox, actor.Outbox, actor.Following, actor.Followers, actor.Summary, actor.Restricted, actor.BoardType)

		if err != nil {
			return activitypub.Actor{}, util.MakeError(err, "CreateNewBoardDB")
		}

		config.Log.Println("board added")

		for _, e := range actor.AuthRequirement {
			query = `insert into actorauth (type, board) values ($1, $2)`
			if _, err := config.DB.Exec(query, e, actor.PreferredUsername); err != nil {
				return activitypub.Actor{}, util.MakeError(err, "CreateNewBoardDB")
			}
		}

		if actor.Id == config.Domain {
			var verify util.Verify
			verify.Type = "admin"
			verify.Identifier = actor.Id

			if err := actor.CreateVerification(verify); err != nil {
				return activitypub.Actor{}, util.MakeError(err, "CreateNewBoardDB")
			}
		}

		activitypub.CreatePem(actor)

		if actor.PreferredUsername != "main" {
			var nObject activitypub.ObjectBase
			var nActivity activitypub.Activity

			nActor, err := activitypub.GetActorFromDB(config.Domain)

			if err != nil {
				return actor, util.MakeError(err, "CreateNewBoardDB")
			}

			nActivity.AtContext.Context = "https://www.w3.org/ns/activitystreams"
			nActivity.Type = "Follow"
			nActivity.Actor = &nActor
			nActivity.Object = nObject
			mActor, err := activitypub.GetActorFromDB(actor.Id)

			if err != nil {
				return actor, util.MakeError(err, "CreateNewBoardDB")
			}

			nActivity.Object.Actor = mActor.Id
			nActivity.To = append(nActivity.To, actor.Id)

			activityRequest := nActivity.AcceptFollow()

			if _, err := activityRequest.SetActorFollowing(); err != nil {
				return actor, util.MakeError(err, "CreateNewBoardDB")
			}

			if err := activityRequest.MakeRequestInbox(); err != nil {
				return actor, util.MakeError(err, "CreateNewBoardDB")
			}
		}
	}

	return actor, nil
}

func RemovePreviewFromFile(id string) error {
	var href string

	query := `select href from activitystream where id in (select preview from activitystream where id=$1)`
	if err := config.DB.QueryRow(query, id).Scan(&href); err != nil {
		return nil
	}

	href = strings.Replace(href, config.Domain+"/", "", 1)

	if href != "static/notfound.png" {
		if _, err := os.Stat(href); err != nil {
			return util.MakeError(err, "RemovePreviewFromFile")
		}

		err := os.Remove(href)
		return util.MakeError(err, "RemovePreviewFromFile")
	}

	obj := activitypub.ObjectBase{Id: id}
	err := obj.DeletePreview()
	return util.MakeError(err, "RemovePreviewFromFile")
}

// if limit less than 1 return all news items
func GetNews(limit int) ([]NewsItem, error) {
	var news []NewsItem
	var query string

	var rows *sql.Rows
	var err error

	if limit > 0 {
		query = `select title, content, time from newsItem order by time desc limit $1`
		rows, err = config.DB.Query(query, limit)
	} else {
		query = `select title, content, time from newsItem order by time desc`
		rows, err = config.DB.Query(query)
	}

	if err != nil {
		return news, util.MakeError(err, "GetNews")
	}

	defer rows.Close()
	for rows.Next() {
		var content string
		n := NewsItem{}

		if err := rows.Scan(&n.Title, &content, &n.Time); err != nil {
			return news, util.MakeError(err, "GetNews")
		}

		content = strings.ReplaceAll(content, "\n", "<br>")
		n.Content = template.HTML(content)

		news = append(news, n)
	}

	return news, nil
}

func GetNewsItem(timestamp int) (NewsItem, error) {
	var news NewsItem
	var content string

	query := `select title, content, time from newsItem where time=$1 limit 1`
	if err := config.DB.QueryRow(query, timestamp).Scan(&news.Title, &content, &news.Time); err != nil {
		return news, util.MakeError(err, "GetNewsItem")
	}

	content = strings.ReplaceAll(content, "\n", "<br>")
	news.Content = template.HTML(content)

	return news, nil
}

func DeleteNewsItem(timestamp int) error {
	query := `delete from newsItem where time=$1`
	_, err := config.DB.Exec(query, timestamp)

	return util.MakeError(err, "DeleteNewsItem")
}

func WriteNews(news NewsItem) error {
	query := `insert into newsItem (title, content, time) values ($1, $2, $3)`
	_, err := config.DB.Exec(query, news.Title, news.Content, time.Now().Unix())

	return util.MakeError(err, "WriteNews")
}

func AddInstanceToInactive(instance string) error {
	var timeStamp string

	query := `select timestamp from inactive where instance=$1`
	if err := config.DB.QueryRow(query, instance).Scan(&timeStamp); err != nil {
		query := `insert into inactive (instance, timestamp) values ($1, $2)`
		_, err := config.DB.Exec(query, instance, time.Now().UTC().Format(time.RFC3339))

		return util.MakeError(err, "AddInstanceToInactive")
	}

	if !IsInactiveTimestamp(timeStamp) {
		return nil
	}

	query = `delete from follower where follower like $1`
	if _, err := config.DB.Exec(query, "%"+instance+"%"); err != nil {
		return util.MakeError(err, "AddInstanceToInactive")
	}

	err := DeleteInstanceFromInactive(instance)
	return util.MakeError(err, "AddInstanceToInactive")
}

func DeleteInstanceFromInactive(instance string) error {
	query := `delete from inactive where instance=$1`
	_, err := config.DB.Exec(query, instance)

	return util.MakeError(err, "DeleteInstanceFromInactive")
}

func IsInactiveTimestamp(timeStamp string) bool {
	stamp, _ := time.Parse(time.RFC3339, timeStamp)
	return time.Now().UTC().Sub(stamp).Hours() > 48
}

func IsReplyToOP(op string, link string) (string, bool, error) {
	var id string

	if op == link {
		return link, true, nil
	}

	re := regexp.MustCompile(`f(\w+)\-`)
	match := re.FindStringSubmatch(link)

	if len(match) > 0 {
		re := regexp.MustCompile(`(.+)\-`)
		link = re.ReplaceAllString(link, "")
		link = "%" + match[1] + "/" + link
	}

	query := `select id from replies where id like $1 and inreplyto=$2`
	if err := config.DB.QueryRow(query, link, op).Scan(&id); err != nil {
		return op, false, nil
	}

	return id, id != "", nil
}

func GetReplyOP(link string) (string, error) {
	var id string

	query := `select id from replies where id in (select inreplyto from replies where id=$1) and inreplyto=''`
	if err := config.DB.QueryRow(query, link).Scan(&id); err != nil {
		return "", nil
	}

	return id, nil
}

func CheckInactive() {
	for {
		CheckInactiveInstances()
		time.Sleep(24 * time.Hour)
	}
}

func CheckInactiveInstances() (map[string]string, error) {
	var rows *sql.Rows
	var err error

	instances := make(map[string]string)

	query := `select following from following`
	if rows, err = config.DB.Query(query); err != nil {
		return instances, util.MakeError(err, "CheckInactiveInstances")
	}

	defer rows.Close()
	for rows.Next() {
		var instance string

		if err := rows.Scan(&instance); err != nil {
			return instances, util.MakeError(err, "CheckInactiveInstances")
		}

		instances[instance] = instance
	}

	query = `select follower from follower`
	if rows, err = config.DB.Query(query); err != nil {
		return instances, util.MakeError(err, "CheckInactiveInstances")
	}

	defer rows.Close()
	for rows.Next() {
		var instance string

		if err := rows.Scan(&instance); err != nil {
			return instances, util.MakeError(err, "CheckInactiveInstances")
		}

		instances[instance] = instance
	}

	re := regexp.MustCompile(config.Domain + `(.+)?`)

	for _, e := range instances {
		actor, err := activitypub.GetActor(e)

		if err != nil {
			return instances, util.MakeError(err, "CheckInactiveInstances")
		}

		if actor.Id == "" && !re.MatchString(e) {
			if err := AddInstanceToInactive(e); err != nil {
				return instances, util.MakeError(err, "CheckInactiveInstances")
			}
		} else {
			if err := DeleteInstanceFromInactive(e); err != nil {
				return instances, util.MakeError(err, "CheckInactiveInstances")
			}
		}
	}

	return instances, nil
}

func GetAdminAuth() (string, string, error) {
	var code string
	var identifier string

	query := `select identifier, code from boardaccess where board=$1 and type='admin'`
	if err := config.DB.QueryRow(query, config.Domain).Scan(&identifier, &code); err != nil {
		return "", "", nil
	}

	return code, identifier, nil
}

func IsHashBanned(hash string) (bool, error) {
	var h string

	query := `select hash from bannedmedia where hash=$1`
	_ = config.DB.QueryRow(query, hash).Scan(&h)

	return h == hash, nil
}

func IsIPBanned(i string) (string, string, time.Time, time.Time, error) {
	var ip string
	var reason string
	var date time.Time
	var expires time.Time

	// Worth also including NULL values just incase?
	query := `select ip, reason, date, expires from bannedips where $1 <<= ip AND expires > now() ORDER BY "expires" DESC;`
	_ = config.DB.QueryRow(query, i).Scan(&ip, &reason, &date, &expires)

	return ip, reason, date, expires, nil
}

func GetAllBansForIP(ip string) ([]Ban, error) {
	var bans []Ban

	// Display permanent bans at top, else sort by date
	query := `SELECT ip, reason, date, expires FROM bannedips where $1 <<= ip ORDER BY (CASE WHEN expires ='9999-12-31 00:00:00' then '1' else '2' END) ASC, date DESC;`
	rows, err := config.DB.Query(query, ip)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var ban Ban
		err := rows.Scan(&ban.IP, &ban.Reason, &ban.Date, &ban.Expires)
		if err != nil {
			return nil, err
		}
		bans = append(bans, ban)
	}

	err = rows.Err()
	if err != nil {
		return nil, err
	}

	return bans, nil
}

func PrintAdminAuth() error {
	code, identifier, err := GetAdminAuth()

	if err != nil {
		return util.MakeError(err, "PrintAdminAuth")
	}

	config.Log.Println("Mod key: " + config.Key)
	config.Log.Println("Admin Login: " + identifier + ", Code: " + code)
	return nil
}

func InitInstance() error {
	if config.InstanceName != "" {
		if _, err := CreateNewBoard(*activitypub.CreateNewActor("", config.InstanceName, config.InstanceSummary, config.AuthReq, false, "")); err != nil {
			return util.MakeError(err, "InitInstance")
		}
	}

	return nil
}

func GetPostIDFromNum(num string) (string, error) {
	var postID string

	query := `select id from activitystream where id like $1`
	if err := config.DB.QueryRow(query, "%"+num).Scan(&postID); err != nil {
		query = `select id from cacheactivitystream where id like $1`
		if err := config.DB.QueryRow(query, "%"+num).Scan(&postID); err != nil {
			return "", util.MakeError(err, "GetPostIDFromNum")
		}
	}

	return postID, nil
}

func IsValidThread(id string) bool {
	var result bool

	query := `select exists
	(select 1 from replies where id = $1 AND inreplyto = '')
	and (exists (select 1 from activitystream where id = $1 and type = 'Note')
	or exists (select 1 from cacheactivitystream where id = $1 and type = 'Note'))`
	config.DB.QueryRow(query, id).Scan(&result)
	return result
}

func GetPostIP(post string) string {
	var ip string
	query := `select ip from identify where id = $1`
	if err := config.DB.QueryRow(query, post).Scan(&ip); err != nil {
		return ""
	}
	if ip == "172.16.0.1" {
		return ""
	}
	return ip
}

func IsTombstone(id string) bool {
	var result bool

	query := `select
	exists (select id from activitystream where id = $1 and type = 'Tombstone')
	or exists (select id from cacheactivitystream where id = $1 and type = 'Tombstone')`
	config.DB.QueryRow(query, id).Scan(&result)

	return result
}

func EnsureMigrationState() error {
	// Check if schema_migrations table exists
	var exists bool
	row := config.DB.QueryRow(`SELECT EXISTS (
		SELECT FROM information_schema.tables 
		WHERE table_name = 'schema_migrations')`)
	if err := row.Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil // migrations already tracked
	}
	// Check if any known table from initial schema exists
	row = config.DB.QueryRow(`SELECT EXISTS (
		SELECT FROM information_schema.tables 
		WHERE table_name = 'actor')`)
	if err := row.Scan(&exists); err != nil {
		return err
	}
	if exists {
		// Mark initial migration as applied
		_, err := config.DB.Exec(`INSERT INTO schema_migrations (version, dirty) VALUES (1, false) ON CONFLICT DO NOTHING`)
		return err
	}
	return nil
}
