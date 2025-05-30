package activitypub

import (
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/anomalous69/fchannel/config"
	"github.com/anomalous69/fchannel/util"
	"github.com/gofiber/fiber/v2"
)

// False positive for application/ld+ld, application/activity+ld, application/json+json
var activityRegexp = regexp.MustCompile(`application/(ld|json|activity)((\+(ld|json))|$)`)

func AcceptActivity(header string) bool {
	accept := false
	if strings.Contains(header, ";") {
		split := strings.Split(header, ";")
		accept = accept || activityRegexp.MatchString(split[0])
		accept = accept || strings.Contains(split[len(split)-1], "profile=\"https://www.w3.org/ns/activitystreams\"")
	} else {
		accept = accept || activityRegexp.MatchString(header)
	}
	return accept
}

func CreateAttachmentObject(file multipart.File, header *multipart.FileHeader) ([]ObjectBase, *os.File, error) {
	contentType, err := util.GetFileContentType(file)
	if err != nil {
		return nil, nil, util.MakeError(err, "CreateAttachmentObject")
	}

	filename := header.Filename
	size := header.Size

	fileType := path.Ext(header.Filename)
	name := fmt.Sprint(time.Now().UTC().Unix())

	tempFile, err := os.Create(fmt.Sprintf("./public/%s%s", name, fileType))
	if err != nil {
		return nil, nil, util.MakeError(err, "CreateAttachmentObject")
	}

	var nAttachment []ObjectBase
	var image ObjectBase

	image.Type = "Attachment"
	image.Name = filename
	image.Href = fmt.Sprintf("%s/public/%s%s", config.Domain, name, fileType)
	image.MediaType = contentType
	image.Size = size
	image.Published = time.Now().UTC()

	nAttachment = append(nAttachment, image)

	return nAttachment, tempFile, nil
}

func CreateNewActor(board string, name string, summary string, authReq []string, restricted bool, boardtype string, optionsmask int) *Actor {
	actor := new(Actor)

	var path string
	if board == "" {
		path = config.Domain
		actor.PreferredUsername = "main"
	} else {
		path = config.Domain + "/" + board
		actor.PreferredUsername = board
	}

	actor.Type = "Group"
	actor.Id = path
	actor.Following = fmt.Sprintf("%s/following", actor.Id)
	actor.Followers = fmt.Sprintf("%s/followers", actor.Id)
	actor.Inbox = fmt.Sprintf("%s/inbox", actor.Id)
	actor.Outbox = fmt.Sprintf("%s/outbox", actor.Id)
	actor.Name = name
	actor.Restricted = restricted
	actor.Summary = summary
	actor.AuthRequirement = authReq
	actor.BoardType = boardtype
	actor.OptionsMask = optionsmask

	return actor
}

func CreateObject(objType string) ObjectBase {
	var nObj ObjectBase

	now := time.Now().UTC()
	nObj.Type = objType
	nObj.Published = now
	nObj.Updated = &now

	return nObj
}

func GetActivityFromJson(ctx *fiber.Ctx) (Activity, error) {

	var respActivity ActivityRaw
	var nActivity Activity
	var nType string

	if err := json.Unmarshal(ctx.Body(), &respActivity); err != nil {
		return nActivity, util.MakeError(err, "GetActivityFromJson")
	}

	if res, err := HasContextFromJson(respActivity.AtContextRaw.Context); err == nil && res {
		var jObj ObjectBase

		if respActivity.Type == "Note" {
			jObj, err = GetObjectFromJson(ctx.Body())
			if err != nil {
				return nActivity, util.MakeError(err, "GetActivityFromJson")
			}

			nType = "Create"
		} else {
			jObj, err = GetObjectFromJson(respActivity.ObjectRaw)
			if err != nil {
				return nActivity, util.MakeError(err, "GetActivityFromJson")
			}

			nType = respActivity.Type
		}

		actor, err := GetActorFromJson(respActivity.ActorRaw)
		if err != nil {
			return nActivity, util.MakeError(err, "GetActivityFromJson")
		}

		to, err := GetToFromJson(respActivity.ToRaw)
		if err != nil {
			return nActivity, util.MakeError(err, "GetActivityFromJson")
		}

		cc, err := GetToFromJson(respActivity.CcRaw)
		if err != nil {
			return nActivity, util.MakeError(err, "GetActivityFromJson")
		}

		nActivity.AtContext.Context = "https://www.w3.org/ns/activitystreams"
		nActivity.Type = nType
		nActivity.Actor = &actor
		nActivity.Published = respActivity.Published
		nActivity.Auth = respActivity.Auth

		if len(to) > 0 {
			nActivity.To = to
		}

		if len(cc) > 0 {
			nActivity.Cc = cc
		}

		nActivity.Name = respActivity.Name
		nActivity.Object = jObj
	} else if err != nil {
		return nActivity, util.MakeError(err, "GetActivityFromJson")
	}

	return nActivity, nil
}

func GetObjectFromJson(obj []byte) (ObjectBase, error) {
	var generic interface{}
	var nObj ObjectBase

	if err := json.Unmarshal(obj, &generic); err != nil {
		return ObjectBase{}, util.MakeError(err, "GetObjectFromJson")
	}

	if generic != nil {
		switch generic.(type) {
		case []interface{}:
			var lObj ObjectBase
			var arrContext ObjectArray

			if err := json.Unmarshal(obj, &arrContext.Object); err != nil {
				return nObj, util.MakeError(err, "GetObjectFromJson")
			}

			if len(arrContext.Object) > 0 {
				lObj = arrContext.Object[0]
			}
			nObj = lObj

		case map[string]interface{}:
			var arrContext Object

			if err := json.Unmarshal(obj, &arrContext.Object); err != nil {
				return nObj, util.MakeError(err, "GetObjectFromJson")
			}

			nObj = *arrContext.Object

		case string:
			var lObj ObjectBase
			var arrContext ObjectString

			if err := json.Unmarshal(obj, &arrContext.Object); err != nil {
				return nObj, util.MakeError(err, "GetObjectFromJson")
			}

			lObj.Id = arrContext.Object
			nObj = lObj
		}
	}

	return nObj, nil
}

func HasContextFromJson(context []byte) (bool, error) {
	var generic interface{}

	err := json.Unmarshal(context, &generic)
	if err != nil {
		return false, util.MakeError(err, "HasContextFromJson")
	}

	hasContext := false

	switch generic.(type) {
	case []interface{}:
		var arrContext AtContextArray
		err = json.Unmarshal(context, &arrContext.Context)
		if len(arrContext.Context) > 0 {
			if arrContext.Context[0] == "https://www.w3.org/ns/activitystreams" {
				hasContext = true
			}
		}

	case string:
		var arrContext AtContextString
		err = json.Unmarshal(context, &arrContext.Context)
		if arrContext.Context == "https://www.w3.org/ns/activitystreams" {
			hasContext = true
		}
	}

	return hasContext, util.MakeError(err, "HasContextFromJson")
}

func GetActorByNameFromDB(name string) (Actor, error) {
	var nActor Actor
	var publicKeyPem string

	query := `select type, id, preferredusername, name, inbox, outbox, following, followers, restricted, summary, publickeypem, boardtype, optionsmask from actor where preferredusername=$1`
	err := config.DB.QueryRow(query, name).Scan(&nActor.Type, &nActor.Id, &nActor.PreferredUsername, &nActor.Name, &nActor.Inbox, &nActor.Outbox, &nActor.Following, &nActor.Followers, &nActor.Restricted, &nActor.Summary, &publicKeyPem, &nActor.BoardType, &nActor.OptionsMask)

	if err != nil {
		return nActor, util.MakeError(err, "GetActorByNameFromDB")
	}

	nActor.PublicKey, err = GetActorPemFromDB(publicKeyPem)

	if err != nil {
		return nActor, util.MakeError(err, "GetActorFromDB")
	}

	if nActor.Id != "" && nActor.PublicKey == nil {
		if err := CreatePublicKeyFromPrivate(&nActor, publicKeyPem); err != nil {
			return nActor, util.MakeError(err, "GetActorByNameFromDB")
		}
	}

	return nActor, nil
}

func GetActorCollectionReq(collection string) (Collection, error) {
	var nCollection Collection

	req, err := http.NewRequest("GET", collection, nil)

	if err != nil {
		return nCollection, util.MakeError(err, "GetActorCollectionReq")
	}

	// TODO: rewrite this for fiber
	pass := "FIXME"
	//_, pass := GetPasswordFromSession(r)

	req.Header.Set("Accept", config.ActivityStreams)

	req.Header.Set("Authorization", "Basic "+pass)

	resp, err := util.RouteProxy(req)

	if err != nil {
		return nCollection, util.MakeError(err, "GetActorCollectionReq")
	}

	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		body, _ := io.ReadAll(resp.Body)

		if err := json.Unmarshal(body, &nCollection); err != nil {
			return nCollection, util.MakeError(err, "GetActorCollectionReq")
		}
	}

	return nCollection, nil
}

func GetActorFollowNameFromPath(path string) string {
	var actor string

	re := regexp.MustCompile(`f\w+-`)
	actor = re.FindString(path)
	actor = strings.Replace(actor, "f", "", 1)
	actor = strings.Replace(actor, "-", "", 1)

	return actor
}

func GetActorFromDB(id string) (Actor, error) {
	var nActor Actor
	var publicKeyPem string

	query := `select type, id, preferredusername, name, inbox, outbox, following, followers, restricted, summary, publickeypem from actor where id=$1`
	err := config.DB.QueryRow(query, id).Scan(&nActor.Type, &nActor.Id, &nActor.PreferredUsername, &nActor.Name, &nActor.Inbox, &nActor.Outbox, &nActor.Following, &nActor.Followers, &nActor.Restricted, &nActor.Summary, &publicKeyPem)

	if err != nil {
		return nActor, util.MakeError(err, "GetActorFromDB")
	}

	nActor.PublicKey, err = GetActorPemFromDB(publicKeyPem)

	if err != nil {
		return nActor, util.MakeError(err, "GetActorFromDB")
	}

	if nActor.Id != "" && nActor.PublicKey.PublicKeyPem == "" {
		if err := CreatePublicKeyFromPrivate(&nActor, publicKeyPem); err != nil {
			return nActor, util.MakeError(err, "GetActorFromDB")
		}
	}

	return nActor, nil
}

func GetActorFromJson(actor []byte) (Actor, error) {
	var generic interface{}
	var nActor Actor
	err := json.Unmarshal(actor, &generic)
	if err != nil {
		return nActor, util.MakeError(err, "GetActorFromJson")
	}

	if generic != nil {
		switch generic.(type) {
		case map[string]interface{}:
			err = json.Unmarshal(actor, &nActor)

		case string:
			var str string
			err = json.Unmarshal(actor, &str)
			nActor.Id = str
		}

		return nActor, util.MakeError(err, "GetActorFromJson")
	}

	return nActor, nil
}

func GetActorsFollowPostFromId(actors []string, id string) (Collection, error) {
	var collection Collection

	for _, e := range actors {
		obj := ObjectBase{Id: e + "/" + id}
		tempCol, err := obj.GetCollectionFromPath()
		if err != nil {
			return collection, util.MakeError(err, "GetActorsFollowPostFromId")
		}

		if len(tempCol.OrderedItems) > 0 {
			collection = tempCol
			return collection, nil
		}
	}

	return collection, nil
}

func GetBoards() ([]Actor, error) {
	var boards []Actor

	query := `select type, id, preferredusername, name, inbox, outbox, following, followers FROM actor`
	rows, err := config.DB.Query(query)

	if err != nil {
		return boards, util.MakeError(err, "GetBoards")
	}

	defer rows.Close()
	for rows.Next() {
		var actor = new(Actor)

		if err := rows.Scan(&actor.Type, &actor.Id, &actor.PreferredUsername, &actor.Name, &actor.Inbox, &actor.Outbox, &actor.Following, &actor.Followers); err != nil {
			return boards, util.MakeError(err, "GetBoards")
		}

		boards = append(boards, *actor)
	}

	return boards, nil
}

func GetToFromJson(to []byte) ([]string, error) {
	var generic interface{}

	if len(to) == 0 {
		return nil, nil
	}

	err := json.Unmarshal(to, &generic)
	if err != nil {
		return nil, util.MakeError(err, "GetToFromJson")
	}

	if generic != nil {
		var nStr []string
		switch generic.(type) {
		case []interface{}:
			err = json.Unmarshal(to, &nStr)
		case string:
			var str string
			err = json.Unmarshal(to, &str)
			nStr = append(nStr, str)
		}
		return nStr, util.MakeError(err, "GetToFromJson")
	}

	return nil, nil
}

func GetActorAndInstance(path string) (string, string) {
	re := regexp.MustCompile(`([@]?([\w\d.-_]+)[@](.+))`)
	atFormat := re.MatchString(path)

	if atFormat {
		match := re.FindStringSubmatch(path)
		if len(match) > 2 {
			return match[2], match[3]
		}
	}

	re = regexp.MustCompile(`(https?://)(www)?([\w\d-_.:]+)(/|\s+|\r|\r\n)?$`)
	mainActor := re.MatchString(path)
	if mainActor {
		match := re.FindStringSubmatch(path)
		if len(match) > 2 {
			return "main", match[3]
		}
	}

	re = regexp.MustCompile(`(https?://)?(www)?([\w\d-_.:]+)\/([\w\d-_.]+)(\/([\w\d-_.]+))?`)
	httpFormat := re.MatchString(path)

	if httpFormat {
		match := re.FindStringSubmatch(path)
		if len(match) > 3 {
			if match[4] == "users" {
				return match[6], match[3]
			}

			return match[4], match[3]
		}
	}

	return "", ""
}
