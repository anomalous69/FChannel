package activitypub

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/anomalous69/fchannel/config"
	"github.com/anomalous69/fchannel/util"
)

type Webfinger struct {
	Subject string          `json:"subject,omitempty"`
	Links   []WebfingerLink `json:"links,omitempty"`
}

var Boards []Board
var FollowingBoards []ObjectBase

type WebfingerLink struct {
	Rel  string `json:"rel,omitempty"`
	Type string `json:"type,omitempty"`
	Href string `json:"href,omitempty"`
}

type Board struct {
	Name        string
	Actor       Actor
	Summary     string
	PrefName    string
	InReplyTo   string
	Location    string
	To          string
	RedirectTo  string
	Captcha     string
	CaptchaCode string
	ModCred     string
	Domain      string
	TP          string
	Restricted  bool
	Post        ObjectBase
	BoardType   string
	OptionsMask int
}
type BoardSortAsc []Board

func (a BoardSortAsc) Len() int           { return len(a) }
func (a BoardSortAsc) Less(i, j int) bool { return a[i].PrefName < a[j].PrefName }
func (a BoardSortAsc) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }

func GetActor(id string) (Actor, error) {
	var respActor Actor

	if id == "" {
		return respActor, nil
	}

	actor, instance := GetActorAndInstance(id)

	if ActorCache[actor+"@"+instance].Id != "" {
		respActor = ActorCache[actor+"@"+instance]
		return respActor, nil
	}

	req, err := http.NewRequest("GET", strings.TrimSpace(id), nil)
	if err != nil {
		return respActor, util.MakeError(err, "GetActor")
	}

	req.Header.Set("Accept", config.ActivityStreams)

	resp, err := util.RouteProxy(req)

	if err != nil {
		return respActor, util.MakeError(err, "GetActor")
	}

	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if err := json.Unmarshal(body, &respActor); err != nil {
		return respActor, util.MakeError(err, "GetActor")
	}

	ActorCache[actor+"@"+instance] = respActor

	return respActor, nil
}

// looks for actor with pattern of board@instance
func FingerActor(path string) (Actor, error) {
	var nActor Actor

	actor, instance := GetActorAndInstance(path)

	if actor == "" && instance == "" {
		return nActor, nil
	}

	if ActorCache[actor+"@"+instance].Id != "" {
		nActor = ActorCache[actor+"@"+instance]
	} else {
		resp, err := FingerRequest(actor, instance)
		if err != nil {
			return nActor, util.MakeError(err, "FingerActor finger request")
		}

		if resp != nil && resp.StatusCode == 200 {
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return nActor, util.MakeError(err, "FingerActor read resp")
			}

			if err := json.Unmarshal(body, &nActor); err != nil {
				return nActor, util.MakeError(err, "FingerActor unmarshal")
			}

			ActorCache[actor+"@"+instance] = nActor
		}
	}

	return nActor, nil
}

func FingerRequest(actor string, instance string) (*http.Response, error) {
	acct := "acct:" + actor + "@" + instance

	// TODO: respect https
	req, err := http.NewRequest("GET", "http://"+instance+"/.well-known/webfinger?resource="+acct, nil)

	if err != nil {
		return nil, util.MakeError(err, "FingerRequest")
	}

	resp, err := util.RouteProxy(req)
	if err != nil {
		return resp, err
	}

	var finger Webfinger

	if resp.StatusCode == 200 {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return resp, err
		}

		if err := json.Unmarshal(body, &finger); err != nil {
			return resp, util.MakeError(err, "FingerRequest")
		}
	}

	if len(finger.Links) > 0 {
		for _, e := range finger.Links {
			if e.Type == "application/activity+json" {
				req, err = http.NewRequest("GET", e.Href, nil)

				if err != nil {
					return resp, util.MakeError(err, "FingerRequest")
				}

				break
			}
		}
	}

	req.Header.Set("Accept", config.ActivityStreams)
	if resp, err = util.RouteProxy(req); err != nil {
		return resp, util.MakeError(err, "FingerRequest")
	}

	return resp, nil
}

func AddInstanceToIndexDB(actor string) error {
	// TODO: completely disabling this until it is actually reasonable to turn it on
	// only actually allow this when it more or less works, i.e. can post, make threads, manage boards, etc
	return nil

	//sleep to be sure the webserver is fully initialized
	//before making finger request
	time.Sleep(15 * time.Second)

	nActor, err := FingerActor(actor)
	if err != nil {
		return util.MakeError(err, "IsValidActor")
	}

	if nActor.Id == "" {
		return nil
	}

	// TODO: maybe allow different indexes?
	reqActivity := Activity{Id: "https://fchan.xyz/followers"}
	followers, err := reqActivity.GetCollection()
	if err != nil {
		return util.MakeError(err, "IsValidActor")
	}

	var alreadyIndex = false
	for _, e := range followers.Items {
		if e.Id == nActor.Id {
			alreadyIndex = true
		}
	}

	if !alreadyIndex {
		actor := Actor{Id: "https://fchan.xyz"}
		return actor.AddFollower(nActor.Id)
	}

	return nil
}

func GetActorByNameFromBoardCollection(name string) Actor {
	var actor Actor
	boards, _ := GetBoardCollection()
	for _, e := range boards {
		if e.Actor.PreferredUsername == name {
			actor = e.Actor
		}
	}
	return actor
}
func GetBoardCollection() ([]Board, error) {
	var collection []Board
	for _, e := range FollowingBoards {
		var board Board
		boardActor, err := GetActorFromDB(e.Id)
		if err != nil {
			return collection, util.MakeError(err, "GetBoardCollection")
		}
		if boardActor.Id == "" {
			boardActor, err = FingerActor(e.Id)
			if err != nil {
				return collection, util.MakeError(err, "GetBoardCollection")
			}
		}
		board.Name = boardActor.Name
		board.PrefName = boardActor.PreferredUsername
		board.Location = "/" + boardActor.PreferredUsername
		board.Actor = boardActor
		board.Restricted = boardActor.Restricted
		collection = append(collection, board)
	}
	sort.Sort(BoardSortAsc(collection))
	return collection, nil
}
func GetActorFromPath(location string, prefix string) (Actor, error) {
	var actor string
	pattern := fmt.Sprintf("%s([^/\n]+)(/.+)?", prefix)
	re := regexp.MustCompile(pattern)
	match := re.FindStringSubmatch(location)
	if len(match) < 1 {
		actor = "/"
	} else {
		actor = strings.Replace(match[1], "/", "", -1)
	}
	if actor == "/" || actor == "outbox" || actor == "inbox" || actor == "following" || actor == "followers" {
		actor = "main"
	}
	var nActor Actor
	nActor, err := GetActorByNameFromDB(actor)
	if err != nil {
		return nActor, util.MakeError(err, "GetActorFromPath")
	}
	if nActor.Id == "" {
		nActor = GetActorByNameFromBoardCollection(actor)
	}
	return nActor, nil
}
func StartupArchive() error {
	for _, e := range FollowingBoards {
		actor, err := GetActorFromDB(e.Id)
		if err != nil {
			return util.MakeError(err, "StartupArchive")
		}
		if err := actor.ArchivePosts(); err != nil {
			return util.MakeError(err, "StartupArchive")
		}
	}
	return nil
}
