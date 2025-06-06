package activitypub

import (
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"fmt"
	"net/smtp"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/anomalous69/fchannel/config"
	"github.com/anomalous69/fchannel/util"
)

func (obj ObjectBase) WantToCache(actor Actor) (bool, error) {
	reqActivity := Activity{Id: obj.Actor + "/followers"}
	objFollowers, err := reqActivity.GetCollection()

	if err != nil {
		return false, util.MakeError(err, "WantToCache")
	}

	actorFollowing, err := actor.GetFollowing()

	if err != nil {
		return false, util.MakeError(err, "WantToCache")
	}

	isOP, _ := obj.CheckIfOP()

	for _, e := range objFollowers.Items {
		if e.Id == actor.Id {
			return true, nil
		}

		for _, k := range actorFollowing {
			if e.Id == k.Id && !isOP && obj.InReplyTo[0].Id != "" {
				return true, nil
			}
		}
	}

	return false, nil
}

func (obj ObjectBase) CreateActivity(activityType string) (Activity, error) {
	var newActivity Activity

	actor, err := FingerActor(obj.Actor)
	if err != nil {
		return newActivity, util.MakeError(err, "CreateActivity")
	}

	newActivity.AtContext.Context = "https://www.w3.org/ns/activitystreams"
	newActivity.Type = activityType
	newActivity.Published = obj.Published
	newActivity.Actor = &actor
	newActivity.Object = obj

	for _, e := range obj.To {
		if obj.Actor != e {
			newActivity.To = append(newActivity.To, e)
		}
	}

	for _, e := range obj.Cc {
		if obj.Actor != e {
			newActivity.Cc = append(newActivity.Cc, e)
		}
	}

	return newActivity, nil
}

func (obj ObjectBase) CheckIfOP() (bool, error) {
	var id string

	query := `select id from replies where inreplyto='' and id=$1 `
	if err := config.DB.QueryRow(query, obj.Id).Scan(&id); err != nil {
		return false, nil
	}

	return true, nil
}

func (obj ObjectBase) GetOP() (string, error) {
	var id string

	query := `select id from replies where inreplyto='' and id in (select inreplyto from replies where id=$1)`

	if err := config.DB.QueryRow(query, obj.Id).Scan(&id); err != nil {
		return obj.Id, nil
	}

	return id, nil
}

func (obj ObjectBase) CreatePreview() *NestedObjectBase {
	var nPreview NestedObjectBase

	re := regexp.MustCompile(`/.+$`)
	mimetype := re.ReplaceAllString(obj.MediaType, "")

	if mimetype != "image" {
		return &nPreview
	}

	re = regexp.MustCompile(`.+/`)
	file := re.ReplaceAllString(obj.MediaType, "")
	href := util.GetUniqueFilename(file)

	nPreview.Type = "Preview"
	nPreview.Name = obj.Name
	nPreview.Href = config.Domain + "" + href
	nPreview.MediaType = obj.MediaType
	nPreview.Size = obj.Size
	nPreview.Published = obj.Published

	re = regexp.MustCompile(`/public/.+`)
	objFile := re.FindString(obj.Href)

	var cmd *exec.Cmd
	switch obj.MediaType {
	case "image/gif":
		cmd = exec.Command(util.MagickBinary, "."+objFile, "-coalesce", "-scale", "250x250>", "+dither", "-remap", "."+objFile+"[0]", "-layers", "Optimize", "-strip", "."+href)
	default:
		cmd = exec.Command(util.MagickBinary, "."+objFile, "-resize", "250x250>", "-strip", "."+href)
	}

	if err := cmd.Run(); err != nil {
		// TODO: previously we would call CheckError here
		var preview NestedObjectBase
		return &preview
	}

	return &nPreview
}

func (obj ObjectBase) DeleteAndRepliesRequest() error {
	activity, err := obj.CreateActivity("Delete")

	if err != nil {
		return util.MakeError(err, "DeleteAndRepliesRequest")
	}

	nObj, err := obj.GetCollectionFromPath()
	if err != nil {
		return util.MakeError(err, "DeleteAndRepliesRequest")
	}

	activity.Actor.Id = nObj.OrderedItems[0].Actor
	activity.Object = nObj.OrderedItems[0]
	objActor, _ := GetActor(nObj.OrderedItems[0].Actor)
	followers, err := objActor.GetFollower()

	if err != nil {
		return util.MakeError(err, "DeleteAndRepliesRequest")
	}
	for _, e := range followers {
		activity.To = append(activity.To, e.Id)
	}

	following, err := objActor.GetFollowing()
	if err != nil {
		return util.MakeError(err, "DeleteAndRepliesRequest")
	}

	for _, e := range following {
		if !util.IsInStringArray(activity.To, e.Id) {
			activity.To = append(activity.To, e.Id)
		}
	}

	err = activity.MakeRequestInbox()

	return util.MakeError(err, "DeleteAndRepliesRequest")
}

// TODO break this off into seperate for Cache
func (obj ObjectBase) DeleteAttachment() error {
	query := `delete from activitystream where id in (select attachment from activitystream where id=$1)`
	if _, err := config.DB.Exec(query, obj.Id); err != nil {
		return util.MakeError(err, "DeleteAttachment")
	}

	query = `delete from cacheactivitystream where id in (select attachment from cacheactivitystream where id=$1)`
	_, err := config.DB.Exec(query, obj.Id)
	return util.MakeError(err, "DeleteAttachment")
}

func (obj ObjectBase) DeleteAttachmentFromFile() error {
	var href string

	query := `select href from activitystream where id in (select attachment from activitystream where id=$1)`
	if err := config.DB.QueryRow(query, obj.Id).Scan(&href); err != nil {
		return nil
	}

	href = strings.Replace(href, config.Domain+"/", "", 1)
	//TODO: Create a deleted placeholder image
	if href != "static/deleted.png" {
		if _, err := os.Stat(href); err != nil {
			return nil
		}
		return os.Remove(href)
	}

	return nil
}

// TODO break this off into seperate for Cache
func (obj ObjectBase) DeletePreview() error {
	query := `delete from activitystream where id=$1`

	if _, err := config.DB.Exec(query, obj.Id); err != nil {
		return util.MakeError(err, "DeletePreview")
	}

	query = `delete from cacheactivitystream where id in (select preview from cacheactivitystream where id=$1)`

	_, err := config.DB.Exec(query, obj.Id)
	return util.MakeError(err, "")
}

func (obj ObjectBase) DeletePreviewFromFile() error {
	var href string

	query := `select href from activitystream where id in (select preview from activitystream where id=$1)`
	if err := config.DB.QueryRow(query, obj.Id).Scan(&href); err != nil {
		return nil
	}

	href = strings.Replace(href, config.Domain+"/", "", 1)
	if href != "static/deleted.png" {
		if _, err := os.Stat(href); err != nil {
			return nil
		}
		return os.Remove(href)
	}

	return nil
}

func (obj ObjectBase) DeleteAll() error {
	if err := obj.DeleteReported(); err != nil {
		return util.MakeError(err, "DeleteAll")
	}

	if err := obj.DeleteAttachmentFromFile(); err != nil {
		return util.MakeError(err, "DeleteAll")
	}

	if err := obj.DeleteAttachment(); err != nil {
		return util.MakeError(err, "DeleteAll")
	}

	if err := obj.DeletePreviewFromFile(); err != nil {
		return util.MakeError(err, "DeleteAll")
	}

	if err := obj.DeletePreview(); err != nil {
		return util.MakeError(err, "DeleteAll")
	}

	if err := obj.Delete(); err != nil {
		return util.MakeError(err, "DeleteAll")
	}

	return obj.DeleteRepliedTo()
}

// TODO break this off into seperate for Cache
func (obj ObjectBase) Delete() error {
	query := `delete from activitystream where id=$1`
	if _, err := config.DB.Exec(query, obj.Id); err != nil {
		return util.MakeError(err, "Delete")
	}

	query = `delete from cacheactivitystream where id=$1`
	_, err := config.DB.Exec(query, obj.Id)
	return util.MakeError(err, "Delete")
}

func (obj ObjectBase) DeleteInReplyTo() error {
	query := `delete from replies where id in (select id from replies where inreplyto=$1)`
	_, err := config.DB.Exec(query, obj.Id)
	return util.MakeError(err, "DeleteInReplyTo")
}

func (obj ObjectBase) DeleteRepliedTo() error {
	query := `delete from replies where id=$1`
	_, err := config.DB.Exec(query, obj.Id)
	return util.MakeError(err, "DeleteRepliedTo")
}

func (obj ObjectBase) DeleteRequest() error {
	activity, err := obj.CreateActivity("Delete")

	if err != nil {
		return util.MakeError(err, "DeleteRequest")
	}

	nObj, err := obj.GetFromPath()

	if err != nil {
		return util.MakeError(err, "DeleteRequest")
	}

	actor, err := FingerActor(nObj.Actor)

	if err != nil {
		return util.MakeError(err, "DeleteRequest")
	}

	activity.Actor = &actor
	objActor, _ := GetActor(nObj.Actor)
	followers, err := objActor.GetFollower()

	if err != nil {
		return util.MakeError(err, "DeleteRequest")
	}

	for _, e := range followers {
		activity.To = append(activity.To, e.Id)
	}

	following, err := objActor.GetFollowing()
	if err != nil {
		return util.MakeError(err, "DeleteRequest")
	}

	for _, e := range following {
		if !util.IsInStringArray(activity.To, e.Id) {
			activity.To = append(activity.To, e.Id)
		}
	}

	err = activity.MakeRequestInbox()

	return util.MakeError(err, "DeleteRequest")
}

func (obj ObjectBase) DeleteReported() error {
	query := `delete from reported where id=$1`
	_, err := config.DB.Exec(query, obj.Id)

	return util.MakeError(err, "DeleteReported")
}

func (obj ObjectBase) GetCollectionLocal() (Collection, error) {
	var nColl Collection
	var result []ObjectBase

	var rows *sql.Rows
	var err error

	query := `select x.id, x.name, x.alias, x.content, x.type, x.published, x.updated, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (select id, name, alias, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where id=$1 and (type='Note' or type='Archive') union select id, name, alias, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from cacheactivitystream where id=$1 and (type='Note' or type='Archive')) as x`
	if rows, err = config.DB.Query(query, obj.Id); err != nil {
		return nColl, util.MakeError(err, "GetCollectionLocal")
	}

	defer rows.Close()
	for rows.Next() {
		var actor Actor
		var post ObjectBase

		var attch ObjectBase

		var prev NestedObjectBase

		err = rows.Scan(&post.Id, &post.Name, &post.Alias, &post.Content, &post.Type, &post.Published, &post.Updated, &post.AttributedTo, &attch.Id, &prev.Id, &actor.Id, &post.TripCode, &post.Sensitive)

		if err != nil {
			return nColl, util.MakeError(err, "GetCollectionLocal")
		}

		post.Sticky, _ = post.IsSticky()
		post.Locked, _ = post.IsLocked()

		post.Actor = actor.Id

		if post.InReplyTo, err = post.GetInReplyTo(); err != nil {
			return nColl, util.MakeError(err, "GetCollectionLocal")
		}

		if post.Replies, err = post.GetReplies(); err != nil {
			return nColl, util.MakeError(err, "GetCollectionLocal")
		}

		if post.Replies != nil {
			var postCnt int
			var imgCnt int

			if postCnt, imgCnt, err = post.GetRepliesCount(); err != nil {
				return nColl, util.MakeError(err, "GetCollectionLocal")
			}

			post.Replies.TotalItems += postCnt
			post.Replies.TotalImgs += imgCnt
		}

		if attch.Id != "" {
			post.Attachment, err = attch.GetAttachment()
			if err != nil {
				return nColl, util.MakeError(err, "GetCollectionLocal")
			}
		}

		if prev.Id != "" {
			if post.Preview, err = prev.GetPreview(); err != nil {
				return nColl, util.MakeError(err, "GetCollectionLocal")
			}
		}

		result = append(result, post)
	}

	nColl.AtContext.Context = "https://www.w3.org/ns/activitystreams"

	nColl.Actor = &Actor{Id: obj.Id}

	nColl.OrderedItems = result

	return nColl, nil
}

func (obj ObjectBase) GetInReplyTo() ([]ObjectBase, error) {
	var result []ObjectBase

	query := `select inreplyto from replies where id =$1`
	rows, err := config.DB.Query(query, obj.Id)

	if err != nil {
		return result, util.MakeError(err, "GetInReplyTo")
	}

	defer rows.Close()
	for rows.Next() {
		var post ObjectBase
		if err := rows.Scan(&post.Id); err != nil {
			return result, util.MakeError(err, "GetInReplyTo")
		}

		result = append(result, post)
	}

	return result, nil
}

// TODO does attachemnts need to be an array in the activitypub structs?
func (obj ObjectBase) GetAttachment() ([]ObjectBase, error) {
	var attachments []ObjectBase
	var attachment ObjectBase

	query := `select x.id, x.type, x.name, x.href, x.mediatype, x.size, x.published from (select id, type, name, href, mediatype, size, published from activitystream where id=$1 union select id, type, name, href, mediatype, size, published from cacheactivitystream where id=$1) as x`
	_ = config.DB.QueryRow(query, obj.Id).Scan(&attachment.Id, &attachment.Type, &attachment.Name, &attachment.Href, &attachment.MediaType, &attachment.Size, &attachment.Published)

	attachments = append(attachments, attachment)
	return attachments, nil
}

func (obj ObjectBase) GetCollectionFromPath() (Collection, error) {
	var nColl Collection
	var result []ObjectBase

	var post ObjectBase
	var actor Actor

	var attch ObjectBase

	var prev NestedObjectBase

	var err error

	query := `select x.id, x.name, x.alias, x.content, x.type, x.published, x.updated, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (select id, name, alias, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where id like $1 and (type='Note' or type='Archive') union select id, name, alias, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from cacheactivitystream where id like $1 and (type='Note' or type='Archive')) as x order by x.updated`
	if err = config.DB.QueryRow(query, obj.Id).Scan(&post.Id, &post.Name, &post.Alias, &post.Content, &post.Type, &post.Published, &post.Updated, &post.AttributedTo, &attch.Id, &prev.Id, &actor.Id, &post.TripCode, &post.Sensitive); err != nil {
		return nColl, err
	}

	post.Sticky, _ = post.IsSticky()
	post.Locked, _ = post.IsLocked()

	post.Actor = actor.Id

	if post.InReplyTo, err = post.GetInReplyTo(); err != nil {
		return nColl, util.MakeError(err, "GetCollectionFromPath")
	}

	if post.Replies, err = post.GetReplies(); err != nil {
		return nColl, util.MakeError(err, "GetCollectionFromPath")
	}

	if attch.Id != "" {
		post.Attachment, err = attch.GetAttachment()
		if err != nil {
			return nColl, util.MakeError(err, "GetCollectionFromPath")
		}
	}

	if prev.Id != "" {
		if post.Preview, err = prev.GetPreview(); err != nil {
			return nColl, util.MakeError(err, "GetCollectionFromPath")
		}
	}

	result = append(result, post)

	nColl.AtContext.Context = "https://www.w3.org/ns/activitystreams"

	nColl.Actor = &Actor{Id: post.Actor}

	nColl.OrderedItems = result

	return nColl, nil
}

func (obj ObjectBase) GetFromPath() (ObjectBase, error) {
	var post ObjectBase

	var attch ObjectBase

	var prev NestedObjectBase

	query := `select id, name, alias, content, type, published, attributedto, attachment, preview, actor from activitystream where id=$1 order by published desc`
	err := config.DB.QueryRow(query, obj.Id).Scan(&post.Id, &post.Name, &post.Alias, &post.Content, &post.Type, &post.Published, &post.AttributedTo, &attch.Id, &prev.Id, &post.Actor)

	if err != nil {
		return post, util.MakeError(err, "GetFromPath")
	}

	post.Replies, err = post.GetReplies()
	if err != nil {
		return post, util.MakeError(err, "GetFromPath")
	}

	if post.Replies != nil {
		var postCnt int
		var imgCnt int

		postCnt, imgCnt, err = post.GetRepliesCount()
		if err != nil {
			return post, util.MakeError(err, "GetFromPath")
		}

		post.Replies.TotalItems += postCnt
		post.Replies.TotalImgs += imgCnt
	}

	if attch.Id != "" {
		post.Attachment, err = attch.GetAttachment()
		if err != nil {
			return post, util.MakeError(err, "GetFromPath")
		}
	}

	if prev.Id != "" {
		post.Preview, err = prev.GetPreview()
		if err != nil {
			return post, util.MakeError(err, "GetFromPath")
		}
	}

	return post, util.MakeError(err, "GetFromPath")
}

func (obj NestedObjectBase) GetPreview() (*NestedObjectBase, error) {
	var preview NestedObjectBase

	query := `select x.id, x.type, x.name, x.href, x.mediatype, x.size, x.published from (select id, type, name, href, mediatype, size, published from activitystream where id=$1 union select id, type, name, href, mediatype, size, published from cacheactivitystream where id=$1) as x`
	if err := config.DB.QueryRow(query, obj.Id).Scan(&preview.Id, &preview.Type, &preview.Name, &preview.Href, &preview.MediaType, &preview.Size, &preview.Published); err != nil {
		return nil, err
	}
	if preview.Id == "" {
		return nil, nil
	}
	return &preview, nil
}

func (obj ObjectBase) GetRepliesCount() (int, int, error) {
	var countId int
	var countImg int

	query := `select count(x.id) over(), sum(case when RTRIM(x.attachment) = '' then 0 else 1 end) over() from (select id, attachment from activitystream where id in (select id from replies where inreplyto=$1) and type='Note' union select id, attachment from cacheactivitystream where id in (select id from replies where inreplyto=$1) and type='Note') as x`

	if err := config.DB.QueryRow(query, obj.Id).Scan(&countId, &countImg); err != nil {
		return 0, 0, nil
	}

	return countId, countImg, nil
}

func (obj ObjectBase) GetReplies() (*CollectionBase, error) {
	var result []ObjectBase

	var postCount int
	var attachCount int

	var rows *sql.Rows
	var err error

	query := `select count(x.id) over(), sum(case when RTRIM(x.attachment) = '' then 0 else 1 end) over(), x.id, x.name, x.alias, x.content, x.type, x.published, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (select * from activitystream where id in (select id from replies where inreplyto=$1) and (type='Note' or type='Archive') union select * from cacheactivitystream where id in (select id from replies where inreplyto=$1) and (type='Note' or type='Archive')) as x order by x.published asc`
	if rows, err = config.DB.Query(query, obj.Id); err != nil {
		return nil, util.MakeError(err, "GetReplies")
	}

	defer rows.Close()
	for rows.Next() {
		var post ObjectBase
		var actor Actor

		var attch ObjectBase

		var prev NestedObjectBase

		err = rows.Scan(&postCount, &attachCount, &post.Id, &post.Name, &post.Alias, &post.Content, &post.Type, &post.Published, &post.AttributedTo, &attch.Id, &prev.Id, &actor.Id, &post.TripCode, &post.Sensitive)

		if err != nil {
			return nil, util.MakeError(err, "GetReplies")
		}

		post.InReplyTo = append(post.InReplyTo, obj)

		post.Actor = actor.Id

		post.Replies, err = post.GetRepliesReplies()

		if err != nil {
			return nil, util.MakeError(err, "GetReplies")
		}

		if attch.Id != "" {
			post.Attachment, err = attch.GetAttachment()
			if err != nil {
				return nil, util.MakeError(err, "GetReplies")
			}
		}

		if prev.Id != "" {
			post.Preview, err = prev.GetPreview()
			if err != nil {
				return nil, util.MakeError(err, "GetReplies")
			}
		}

		result = append(result, post)
	}

	if postCount == 0 {
		return nil, nil
	}

	return &CollectionBase{
		OrderedItems: result,
		TotalItems:   postCount,
		TotalImgs:    attachCount,
	}, nil
}

func (obj ObjectBase) GetRepliesLimit(limit int) (*CollectionBase, error) {
	var result []ObjectBase

	var postCount int
	var attachCount int

	var rows *sql.Rows
	var err error

	query := `select count(x.id) over(), sum(case when RTRIM(x.attachment) = '' then 0 else 1 end) over(), x.id, x.name, x.alias, x.content, x.type, x.published, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (select * from activitystream where id in (select id from replies where inreplyto=$1) and type='Note' union select * from cacheactivitystream where id in (select id from replies where inreplyto=$1) and type='Note') as x order by x.published desc limit $2`
	if rows, err = config.DB.Query(query, obj.Id, limit); err != nil {
		return nil, util.MakeError(err, "GetRepliesLimit")
	}

	defer rows.Close()
	for rows.Next() {
		var post ObjectBase
		var actor Actor

		var attch ObjectBase

		var prev NestedObjectBase

		err = rows.Scan(&postCount, &attachCount, &post.Id, &post.Name, &post.Alias, &post.Content, &post.Type, &post.Published, &post.AttributedTo, &attch.Id, &prev.Id, &actor.Id, &post.TripCode, &post.Sensitive)

		if err != nil {
			return nil, util.MakeError(err, "GetRepliesLimit")
		}

		post.InReplyTo = append(post.InReplyTo, obj)

		post.Actor = actor.Id

		post.Replies, err = post.GetRepliesReplies()

		if err != nil {
			return nil, util.MakeError(err, "GetRepliesLimit")
		}

		if attch.Id != "" {
			post.Attachment, err = attch.GetAttachment()
			if err != nil {
				return nil, util.MakeError(err, "GetRepliesLimit")
			}
		}

		if prev.Id != "" {
			post.Preview, err = prev.GetPreview()
			if err != nil {
				return nil, util.MakeError(err, "GetRepliesLimit")
			}
		}

		result = append(result, post)
	}

	if postCount == 0 {
		return nil, nil
	}

	sort.Sort(ObjectBaseSortAsc(result))

	return &CollectionBase{
		OrderedItems: result,
		TotalItems:   postCount,
		TotalImgs:    attachCount,
	}, nil
}

func (obj ObjectBase) GetRepliesReplies() (*CollectionBase, error) {
	var result []ObjectBase

	var postCount int
	var attachCount int

	var err error
	var rows *sql.Rows

	query := `select count(x.id) over(), sum(case when RTRIM(x.attachment) = '' then 0 else 1 end) over(), x.id, x.name, x.alias, x.content, x.type, x.published, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (select * from activitystream where id in (select id from replies where inreplyto=$1) and (type='Note' or type='Archive') union select * from cacheactivitystream where id in (select id from replies where inreplyto=$1) and (type='Note' or type='Archive')) as x order by x.published asc`
	if rows, err = config.DB.Query(query, obj.Id); err != nil {
		return nil, util.MakeError(err, "GetRepliesReplies")
	}

	defer rows.Close()
	for rows.Next() {

		var post ObjectBase
		var actor Actor

		var attch ObjectBase

		var prev NestedObjectBase

		err = rows.Scan(&postCount, &attachCount, &post.Id, &post.Name, &post.Alias, &post.Content, &post.Type, &post.Published, &post.AttributedTo, &attch.Id, &prev.Id, &actor.Id, &post.TripCode, &post.Sensitive)
		if err != nil {
			return nil, util.MakeError(err, "GetRepliesReplies")
		}

		post.InReplyTo = append(post.InReplyTo, obj)

		post.Actor = actor.Id

		if attch.Id != "" {
			post.Attachment, err = attch.GetAttachment()
			if err != nil {
				return nil, util.MakeError(err, "GetRepliesReplies")
			}
		}

		if prev.Id != "" {
			post.Preview, err = prev.GetPreview()
			if err != nil {
				return nil, util.MakeError(err, "GetRepliesReplies")
			}
		}

		result = append(result, post)
	}

	if postCount == 0 {
		return nil, nil
	}

	return &CollectionBase{
		OrderedItems: result,
		TotalItems:   postCount,
		TotalImgs:    attachCount,
	}, nil
}

func (obj ObjectBase) GetType() (string, error) {
	var nType string

	query := `select type from activitystream where id=$1 union select type from cacheactivitystream where id=$1`
	if err := config.DB.QueryRow(query, obj.Id).Scan(&nType); err != nil {
		return "", nil
	}

	return nType, nil
}

func (obj ObjectBase) IsCached() (bool, error) {
	var nID string

	query := `select id from cacheactivitystream where id=$1`
	if err := config.DB.QueryRow(query, obj.Id).Scan(&nID); err != nil {
		return false, util.MakeError(err, "GetType")
	}

	return true, nil
}

func (obj ObjectBase) IsLocal() (bool, error) {
	var nID string

	query := `select id from activitystream where id=$1`
	if err := config.DB.QueryRow(query, obj.Id).Scan(&nID); err != nil {
		return false, err
	}

	return true, nil
}

func (obj ObjectBase) IsReplyInThread(id string) (bool, error) {
	reqActivity := Activity{Id: obj.InReplyTo[0].Id}
	coll, _, err := reqActivity.CheckValid()

	if err != nil {
		return false, util.MakeError(err, "IsReplyInThread")
	}

	for _, e := range coll.OrderedItems[0].Replies.OrderedItems {
		if e.Id == id {
			return true, nil
		}
	}

	return false, nil
}

// TODO break this off into seperate for Cache
func (obj ObjectBase) MarkSensitive(sensitive bool) error {
	var query = `update activitystream set sensitive=$1 where id=$2`
	if _, err := config.DB.Exec(query, sensitive, obj.Id); err != nil {
		return util.MakeError(err, "MarkSensitive")
	}

	query = `update cacheactivitystream set sensitive=$1 where id=$2`
	_, err := config.DB.Exec(query, sensitive, obj.Id)
	return util.MakeError(err, "MarkSensitive")
}

func (obj ObjectBase) SetAttachmentType(_type string) error {
	datetime := time.Now().UTC().Format(time.RFC3339)

	query := `update activitystream set type=$1, deleted=$2 where id in (select attachment from activitystream where id=$3)`
	if _, err := config.DB.Exec(query, _type, datetime, obj.Id); err != nil {
		return util.MakeError(err, "SetAttachmentType")
	}

	query = `update cacheactivitystream set type=$1, deleted=$2 where id in (select attachment from cacheactivitystream  where id=$3)`
	_, err := config.DB.Exec(query, _type, datetime, obj.Id)
	return util.MakeError(err, "SetAttachmentType")
}

func (obj ObjectBase) SetAttachmentRepliesType(_type string) error {
	datetime := time.Now().UTC().Format(time.RFC3339)

	query := `update activitystream set type=$1, deleted=$2 where id in (select attachment from activitystream where id in (select id from replies where inreplyto=$3))`
	if _, err := config.DB.Exec(query, _type, datetime, obj.Id); err != nil {
		return util.MakeError(err, "SetAttachmentRepliesType")
	}

	query = `update cacheactivitystream set type=$1, deleted=$2 where id in (select attachment from cacheactivitystream where id in (select id from replies where inreplyto=$3))`
	_, err := config.DB.Exec(query, _type, datetime, obj.Id)
	return util.MakeError(err, "SetAttachmentRepliesType")
}

func (obj ObjectBase) SetPreviewType(_type string) error {
	datetime := time.Now().UTC().Format(time.RFC3339)

	query := `update activitystream set type=$1, deleted=$2 where id in (select preview from activitystream where id=$3)`
	if _, err := config.DB.Exec(query, _type, datetime, obj.Id); err != nil {
		return util.MakeError(err, "SetPreviewType")
	}

	query = `update cacheactivitystream set type=$1, deleted=$2 where id in (select preview from cacheactivitystream where id=$3)`
	_, err := config.DB.Exec(query, _type, datetime, obj.Id)
	return util.MakeError(err, "SetPreviewType")
}

func (obj ObjectBase) SetPreviewRepliesType(_type string) error {
	datetime := time.Now().UTC().Format(time.RFC3339)

	query := `update activitystream set type=$1, deleted=$2 where id in (select preview from activitystream where id in (select id from replies where inreplyto=$3))`
	if _, err := config.DB.Exec(query, _type, datetime, obj.Id); err != nil {
		return util.MakeError(err, "SetPreviewRepliesType")
	}

	query = `update cacheactivitystream set type=$1, deleted=$2 where id in (select preview from cacheactivitystream where id in (select id from replies where inreplyto=$3))`
	_, err := config.DB.Exec(query, _type, datetime, obj.Id)
	return util.MakeError(err, "SetPreviewRepliesType")
}

func (obj ObjectBase) SetType(_type string) error {
	if err := obj.SetAttachmentType(_type); err != nil {
		return util.MakeError(err, "SetType")
	}

	if err := obj.SetPreviewType(_type); err != nil {
		return util.MakeError(err, "SetType")
	}

	return obj._SetType(_type)
}

func (obj ObjectBase) _SetType(_type string) error {
	datetime := time.Now().UTC().Format(time.RFC3339)

	query := `update activitystream set type=$1, deleted=$2 where id=$3`
	if _, err := config.DB.Exec(query, _type, datetime, obj.Id); err != nil {
		return util.MakeError(err, "_SetType")
	}

	query = `update cacheactivitystream set type=$1, deleted=$2 where id=$3`
	_, err := config.DB.Exec(query, _type, datetime, obj.Id)
	return util.MakeError(err, "_SetType")
}

func (obj ObjectBase) SetRepliesType(_type string) error {
	if err := obj.SetAttachmentType(_type); err != nil {
		return util.MakeError(err, "SetRepliesType")
	}

	if err := obj.SetPreviewType(_type); err != nil {
		return util.MakeError(err, "SetRepliesType")
	}

	if err := obj._SetRepliesType(_type); err != nil {
		return util.MakeError(err, "SetRepliesType")
	}

	if err := obj.SetAttachmentRepliesType(_type); err != nil {
		return util.MakeError(err, "SetRepliesType")
	}

	if err := obj.SetPreviewRepliesType(_type); err != nil {
		return util.MakeError(err, "SetRepliesType")
	}

	return obj.SetType(_type)
}

func (obj ObjectBase) _SetRepliesType(_type string) error {
	datetime := time.Now().UTC().Format(time.RFC3339)

	var query = `update activitystream set type=$1, deleted=$2 where id in (select id from replies where inreplyto=$3)`
	if _, err := config.DB.Exec(query, _type, datetime, obj.Id); err != nil {
		return util.MakeError(err, "_SetRepliesType")
	}

	query = `update cacheactivitystream set type=$1, deleted=$2 where id in (select id from replies where inreplyto=$3)`
	_, err := config.DB.Exec(query, _type, datetime, obj.Id)
	return util.MakeError(err, "_SetRepliesType")
}

func (obj ObjectBase) TombstoneAttachment() error {
	datetime := time.Now().UTC().Format(time.RFC3339)

	query := `update activitystream set type='Tombstone', mediatype='image/png', href=$1, name='', content='', attributedto='deleted', deleted=$2 where id in (select attachment from activitystream where id=$3)`
	if _, err := config.DB.Exec(query, config.Domain+"/static/notfound.png", datetime, obj.Id); err != nil {
		return util.MakeError(err, "_SetRepliesType")
	}

	query = `update cacheactivitystream set type='Tombstone', mediatype='image/png', href=$1, name='', content='', attributedto='deleted', deleted=$2 where id in (select attachment from cacheactivitystream where id=$3)`
	_, err := config.DB.Exec(query, config.Domain+"/static/notfound.png", datetime, obj.Id)
	return util.MakeError(err, "_SetRepliesType")
}

func (obj ObjectBase) TombstoneAttachmentReplies() error {
	var attachment ObjectBase

	query := `select id from activitystream where id in (select id from replies where inreplyto=$1)`
	if err := config.DB.QueryRow(query, obj.Id).Scan(&attachment.Id); err != nil {
		return nil
	}

	if err := attachment.DeleteAttachmentFromFile(); err != nil {
		return util.MakeError(err, "TombstoneAttachmentReplies")
	}

	if err := attachment.TombstoneAttachment(); err != nil {
		return util.MakeError(err, "TombstoneAttachmentReplies")
	}

	return nil
}

func (obj ObjectBase) TombstonePreview() error {
	datetime := time.Now().UTC().Format(time.RFC3339)

	query := `update activitystream set type='Tombstone', mediatype='image/png', href=$1, name='', content='', attributedto='deleted', deleted=$2 where id in (select preview from activitystream where id=$3)`
	if _, err := config.DB.Exec(query, config.Domain+"/static/notfound.png", datetime, obj.Id); err != nil {
		return util.MakeError(err, "TombstonePreview")
	}

	query = `update cacheactivitystream set type='Tombstone', mediatype='image/png', href=$1, name='', content='', attributedto='deleted', deleted=$2 where id in (select preview from cacheactivitystream where id=$3)`
	_, err := config.DB.Exec(query, config.Domain+"/static/notfound.png", datetime, obj.Id)
	return util.MakeError(err, "TombstonePreview")
}

func (obj ObjectBase) TombstonePreviewReplies() error {
	var attachment ObjectBase

	query := `select id from activitystream where id in (select id from replies where inreplyto=$1)`
	if err := config.DB.QueryRow(query, obj.Id).Scan(&attachment.Id); err != nil {
		return nil
	}

	if err := attachment.DeletePreviewFromFile(); err != nil {
		return util.MakeError(err, "TombstonePreviewReplies")
	}

	if err := attachment.TombstonePreview(); err != nil {
		return util.MakeError(err, "TombstonePreviewReplies")
	}

	return nil
}

func (obj ObjectBase) Tombstone() error {
	if err := obj.DeleteReported(); err != nil {
		return util.MakeError(err, "Tombstone")
	}

	if err := obj.DeleteAttachmentFromFile(); err != nil {
		return util.MakeError(err, "Tombstone")
	}

	if err := obj.TombstoneAttachment(); err != nil {
		return util.MakeError(err, "Tombstone")
	}

	if err := obj.DeletePreviewFromFile(); err != nil {
		return util.MakeError(err, "Tombstone")
	}

	if err := obj.TombstonePreview(); err != nil {
		return util.MakeError(err, "Tombstone")
	}

	return obj._Tombstone()
}

func (obj ObjectBase) _Tombstone() error {
	datetime := time.Now().UTC().Format(time.RFC3339)

	query := `update activitystream set type='Tombstone', name='', content='', attributedto='deleted', tripcode='', deleted=$1 where id=$2`
	if _, err := config.DB.Exec(query, datetime, obj.Id); err != nil {
		return util.MakeError(err, "_Tombstone")
	}

	query = `update cacheactivitystream set type='Tombstone', name='', content='', attributedto='deleted', tripcode='',  deleted=$1 where id=$2`
	_, err := config.DB.Exec(query, datetime, obj.Id)
	return util.MakeError(err, "_Tombstone")
}

func (obj ObjectBase) TombstoneReplies() error {
	if err := obj.DeleteReported(); err != nil {
		return util.MakeError(err, "TombstoneReplies")
	}

	if err := obj.DeleteAttachmentFromFile(); err != nil {
		return util.MakeError(err, "TombstoneReplies")
	}

	if err := obj.TombstoneAttachment(); err != nil {
		return util.MakeError(err, "TombstoneReplies")
	}

	if err := obj.DeletePreviewFromFile(); err != nil {
		return util.MakeError(err, "TombstoneReplies")
	}

	if err := obj.TombstonePreview(); err != nil {
		return util.MakeError(err, "TombstoneReplies")
	}

	if err := obj._TombstoneReplies(); err != nil {
		return util.MakeError(err, "TombstoneReplies")
	}

	if err := obj.TombstoneAttachmentReplies(); err != nil {
		return util.MakeError(err, "TombstoneReplies")
	}

	if err := obj.TombstonePreviewReplies(); err != nil {
		return util.MakeError(err, "TombstoneReplies")
	}

	return obj.Tombstone()
}

func (obj ObjectBase) _TombstoneReplies() error {
	datetime := time.Now().UTC().Format(time.RFC3339)

	query := `update activitystream set type='Tombstone', name='', content='', attributedto='deleted', tripcode='', deleted=$1 where id in (select id from replies where inreplyto=$2)`
	if _, err := config.DB.Exec(query, datetime, obj.Id); err != nil {
		return util.MakeError(err, "_TombstoneReplies")
	}

	query = `update cacheactivitystream set type='Tombstone', name='', content='', attributedto='deleted', tripcode='', deleted=$1 where id in (select id from replies where inreplyto=$2)`
	_, err := config.DB.Exec(query, datetime, obj.Id)
	return util.MakeError(err, "_TombstoneReplies")
}

func (obj ObjectBase) UpdateType(_type string) error {
	query := `update activitystream set type=$2 where id=$1 and type !='Tombstone'`
	if _, err := config.DB.Exec(query, obj.Id, _type); err != nil {
		return util.MakeError(err, "UpdateType")
	}

	query = `update cacheactivitystream set type=$2 where id=$1 and type !='Tombstone'`
	_, err := config.DB.Exec(query, obj.Id, _type)
	return util.MakeError(err, "UpdateType")
}

func (obj ObjectBase) UpdatePreview(preview string) error {
	query := `update activitystream set preview=$1 where attachment=$2`
	_, err := config.DB.Exec(query, preview, obj.Id)
	return util.MakeError(err, "UpdatePreview")
}

func (obj ObjectBase) Write() (ObjectBase, error) {
	id, err := util.CreateUniqueID(obj.Actor)
	if err != nil {
		return obj, util.MakeError(err, "Write")
	}

	obj.Id = fmt.Sprintf("%s/%s", obj.Actor, id)
	// TODO: decide if ID's should be unique per instance
	// TODO: evaluate collisions
	if obj.Actor == config.Domain+"/bint" {
		re := regexp.MustCompile(`id:HiddenID`)
		if !re.MatchString(obj.Alias) {
			var threadid string
			op := len(obj.InReplyTo) - 1
			if op >= 0 {
				if obj.InReplyTo[op].Id == "" {
					threadid = obj.Id
				} else {
					threadid = obj.InReplyTo[0].Id
				}
			}
			input := []byte(obj.Alias + threadid)
			hasher := sha256.New()
			hasher.Write(input)
			sha := base64.URLEncoding.EncodeToString(hasher.Sum(nil))

			r := []rune(sha)
			trunc := r[:8]
			uniqID := string(trunc)

			re = regexp.MustCompile(`id:\S*`)
			obj.Alias = re.ReplaceAllString(obj.Alias, "id:"+uniqID)
		}
	}

	if len(obj.Attachment) > 0 {
		now := time.Now().UTC()
		if obj.Preview.Href != "" {
			id, err := util.CreateUniqueID(obj.Actor)
			if err != nil {
				return obj, util.MakeError(err, "Write")
			}

			obj.Preview.Id = fmt.Sprintf("%s/%s", obj.Actor, id)
			obj.Preview.Published = now
			obj.Preview.Updated = &now
			obj.Preview.AttributedTo = obj.Id
			if err := obj.Preview.WritePreview(); err != nil {
				return obj, util.MakeError(err, "Write")
			}
		}
		for i := range obj.Attachment {
			id, err := util.CreateUniqueID(obj.Actor)
			if err != nil {
				return obj, util.MakeError(err, "Write")
			}

			obj.Attachment[i].Id = fmt.Sprintf("%s/%s", obj.Actor, id)
			obj.Attachment[i].Published = now
			obj.Attachment[i].Updated = &now
			obj.Attachment[i].AttributedTo = obj.Id
			obj.Attachment[i].WriteAttachment()
			obj.WriteWithAttachment(obj.Attachment[i])
		}
	} else {
		if err := obj._Write(); err != nil {
			return obj, util.MakeError(err, "Write")
		}
	}

	err = obj.WriteReply()

	return obj, util.MakeError(err, "Write")
}

func (obj ObjectBase) _Write() error {
	obj.Name = util.EscapeString(obj.Name)
	obj.Content = util.EscapeString(obj.Content)
	obj.AttributedTo = util.EscapeString(obj.AttributedTo)
	obj.Alias = util.EscapeString(obj.Alias)

	query := `insert into activitystream (id, type, name, alias, content, published, updated, attributedto, actor, tripcode, sensitive) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`
	_, err := config.DB.Exec(query, obj.Id, obj.Type, obj.Name, obj.Alias, obj.Content, obj.Published, obj.Updated, obj.AttributedTo, obj.Actor, obj.TripCode, obj.Sensitive)

	return util.MakeError(err, "_Write")
}

func (obj ObjectBase) WriteAttachment() error {
	query := `insert into activitystream (id, type, name, href, published, updated, attributedTo, mediatype, size) values ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	_, err := config.DB.Exec(query, obj.Id, obj.Type, obj.Name, obj.Href, obj.Published, obj.Updated, obj.AttributedTo, obj.MediaType, obj.Size)

	return util.MakeError(err, "WriteAttachment")
}

func (obj ObjectBase) WriteAttachmentCache() error {
	var id string

	query := `select id from cacheactivitystream where id=$1`
	if err := config.DB.QueryRow(query, obj.Id).Scan(&id); err != nil {
		if obj.Updated == nil {
			obj.Updated = &obj.Published
		}

		query = `insert into cacheactivitystream (id, type, name, href, published, updated, attributedTo, mediatype, size) values ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
		_, err = config.DB.Exec(query, obj.Id, obj.Type, obj.Name, obj.Href, obj.Published, obj.Updated, obj.AttributedTo, obj.MediaType, obj.Size)
		return util.MakeError(err, "WriteAttachmentCache")
	}

	return nil
}

func (obj ObjectBase) _WriteCache() error {
	var id string

	obj.Name = util.EscapeString(obj.Name)
	obj.Content = util.EscapeString(obj.Content)
	obj.AttributedTo = util.EscapeString(obj.AttributedTo)

	query := `select id from cacheactivitystream where id=$1`
	if err := config.DB.QueryRow(query, obj.Id).Scan(&id); err != nil {
		if obj.Updated == nil {
			obj.Updated = &obj.Published
		}

		query = `insert into cacheactivitystream (id, type, name, content, published, updated, attributedto, actor, tripcode, sensitive) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`
		_, err = config.DB.Exec(query, obj.Id, obj.Type, obj.Name, obj.Content, obj.Published, obj.Updated, obj.AttributedTo, obj.Actor, obj.TripCode, obj.Sensitive)
		return util.MakeError(err, "_WriteCache")
	}

	return nil
}

func (obj ObjectBase) WriteCacheWithAttachment(attachment ObjectBase) error {
	var id string

	obj.Name = util.EscapeString(obj.Name)
	obj.Content = util.EscapeString(obj.Content)
	obj.AttributedTo = util.EscapeString(obj.AttributedTo)

	query := `select id from cacheactivitystream where id=$1`
	if err := config.DB.QueryRow(query, obj.Id).Scan(&id); err != nil {
		if obj.Updated == nil {
			obj.Updated = &obj.Published
		}

		query = `insert into cacheactivitystream (id, type, name, content, attachment, preview, published, updated, attributedto, actor, tripcode, sensitive) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`
		_, err = config.DB.Exec(query, obj.Id, obj.Type, obj.Name, obj.Content, attachment.Id, obj.Preview.Id, obj.Published, obj.Updated, obj.AttributedTo, obj.Actor, obj.TripCode, obj.Sensitive)
		return util.MakeError(err, "WriteCacheWithAttachment")
	}

	return nil
}

func (obj NestedObjectBase) WritePreview() error {
	query := `insert into activitystream (id, type, name, href, published, updated, attributedTo, mediatype, size) values ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	_, err := config.DB.Exec(query, obj.Id, obj.Type, obj.Name, obj.Href, obj.Published, obj.Updated, obj.AttributedTo, obj.MediaType, obj.Size)
	return util.MakeError(err, "WritePreview")
}

func (obj NestedObjectBase) WritePreviewCache() error {
	var id string

	query := `select id from cacheactivitystream where id=$1`
	err := config.DB.QueryRow(query, obj.Id).Scan(&id)
	if err != nil {
		if obj.Updated == nil {
			obj.Updated = &obj.Published
		}

		query = `insert into cacheactivitystream (id, type, name, href, published, updated, attributedTo, mediatype, size) values ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
		_, err = config.DB.Exec(query, obj.Id, obj.Type, obj.Name, obj.Href, obj.Published, obj.Updated, obj.AttributedTo, obj.MediaType, obj.Size)
		return util.MakeError(err, "WritePreviewCache")
	}

	return nil
}

func (obj ObjectBase) WriteReply() error {
	for i, e := range obj.InReplyTo {
		if isOP, err := obj.CheckIfOP(); !isOP && i == 0 {
			var nObj ObjectBase
			nObj.Id = e.Id

			nType, err := nObj.GetType()
			if err != nil {
				return util.MakeError(err, "WriteReply")
			}

			if nType == "Archive" {
				if err := obj.UpdateType("Archive"); err != nil {
					return util.MakeError(err, "WriteReply")
				}
			}
		} else if err != nil {
			return util.MakeError(err, "WriteReply")
		}

		var id string

		query := `select id from replies where id=$1 and inreplyto=$2`
		if err := config.DB.QueryRow(query, obj.Id, e.Id).Scan(&id); err != nil {
			query := `insert into replies (id, inreplyto) values ($1, $2)`
			if _, err := config.DB.Exec(query, obj.Id, e.Id); err != nil {
				return util.MakeError(err, "WriteReply")
			}
		}

		update := true
		for _, o := range obj.Option {
			if o == "sage" || o == "nokosage" {
				update = false
				break
			}
		}

		if update {
			if err := e.WriteUpdate(obj.Published); err != nil {
				return util.MakeError(err, "WriteReply")
			}
		}
	}

	if len(obj.InReplyTo) == 0 {
		var id string

		query := `select id from replies where id=$1 and inreplyto=''`
		if err := config.DB.QueryRow(query, obj.Id).Scan(&id); err != nil {
			query := `insert into replies (id, inreplyto) values ($1, $2)`
			if _, err := config.DB.Exec(query, obj.Id, ""); err != nil {
				return util.MakeError(err, "WriteReply")
			}
		}
	}

	return nil
}

func (obj ObjectBase) WriteCache() (ObjectBase, error) {
	if isBlacklisted, err, regex := util.IsPostBlacklist(obj.Content); err != nil || isBlacklisted {
		config.Log.Println("Blacklist post blocked \nRegex: " + regex + "\n" + obj.Content)
		return obj, util.MakeError(err, "WriteObjectToCache")
	}

	if len(obj.Attachment) > 0 {
		if obj.Preview.Href != "" {
			obj.Preview.WritePreviewCache()
		}

		for i := range obj.Attachment {
			obj.Attachment[i].WriteAttachmentCache()
			obj.WriteCacheWithAttachment(obj.Attachment[i])
		}
	} else {
		obj._WriteCache()
	}

	obj.WriteReply()

	if obj.Replies != nil {
		for _, e := range obj.Replies.OrderedItems {
			e.WriteCache()
		}
	}

	return obj, nil
}

func (obj ObjectBase) WriteUpdate(updated time.Time) error {
	query := `update activitystream set updated=$1 where id=$2`
	if _, err := config.DB.Exec(query, updated, obj.Id); err != nil {
		return util.MakeError(err, "WriteUpdate")
	}

	query = `update cacheactivitystream set updated=$1 where id=$2`
	_, err := config.DB.Exec(query, updated, obj.Id)
	return util.MakeError(err, "WriteUpdate")
}

func (obj ObjectBase) WriteWithAttachment(attachment ObjectBase) {
	obj.Name = util.EscapeString(obj.Name)
	obj.Content = util.EscapeString(obj.Content)
	obj.AttributedTo = util.EscapeString(obj.AttributedTo)

	query := `insert into activitystream (id, type, name, alias, content, attachment, preview, published, updated, attributedto, actor, tripcode, sensitive) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`
	_, e := config.DB.Exec(query, obj.Id, obj.Type, obj.Name, obj.Alias, obj.Content, attachment.Id, obj.Preview.Id, obj.Published, obj.Updated, obj.AttributedTo, obj.Actor, obj.TripCode, obj.Sensitive)

	if e != nil {
		config.Log.Println("error inserting new activity with attachment")
		panic(e)
	}
}

func (obj ObjectBase) MarkSticky(actorID string) error {
	var count int

	var query = `select count(id) from replies where inreplyto='' and id=$1`
	if err := config.DB.QueryRow(query, obj.Id).Scan(&count); err != nil {
		return util.MakeError(err, "MarkSticky")
	}

	if count == 1 {
		var nCount int
		query = `select count(activity_id) from sticky where activity_id=$1`
		if err := config.DB.QueryRow(query, obj.Id).Scan(&nCount); err != nil {
			return util.MakeError(err, "MarkSticky")
		}

		if nCount > 0 {
			query = `delete from sticky where activity_id=$1`
			if _, err := config.DB.Exec(query, obj.Id); err != nil {
				return util.MakeError(err, "MarkSticky")
			}
		} else {
			query = `insert into sticky (actor_id, activity_id) values ($1, $2)`
			if _, err := config.DB.Exec(query, actorID, obj.Id); err != nil {
				return util.MakeError(err, "MarkSticky")
			}
		}
	}

	return nil
}

func (obj ObjectBase) MarkLocked(actorID string) error {
	var count int

	var query = `select count(id) from replies where inreplyto='' and id=$1`
	if err := config.DB.QueryRow(query, obj.Id).Scan(&count); err != nil {
		return util.MakeError(err, "MarkLocked")
	}

	if count == 1 {
		var nCount int

		query = `select count(activity_id) from locked where activity_id=$1`
		if err := config.DB.QueryRow(query, obj.Id).Scan(&nCount); err != nil {
			return util.MakeError(err, "MarkLocked")
		}

		if nCount > 0 {
			query = `delete from locked where activity_id=$1`
			if _, err := config.DB.Exec(query, obj.Id); err != nil {
				return util.MakeError(err, "MarkLocked")
			}
		} else {
			query = `insert into locked (actor_id, activity_id) values ($1, $2)`
			if _, err := config.DB.Exec(query, actorID, obj.Id); err != nil {
				return util.MakeError(err, "MarkLocked")
			}
		}
	}

	return nil
}

func (obj ObjectBase) IsSticky() (bool, error) {
	var count int

	query := `select count(activity_id) from sticky where activity_id=$1 `
	if err := config.DB.QueryRow(query, obj.Id).Scan(&count); err != nil {
		return false, util.MakeError(err, "IsSticky")
	}

	if count != 0 {
		return true, nil
	}

	return false, nil
}

func (obj ObjectBase) IsLocked() (bool, error) {
	var count int

	query := `select count(activity_id) from locked where activity_id=$1 `
	if err := config.DB.QueryRow(query, obj.Id).Scan(&count); err != nil {
		return false, util.MakeError(err, "IsSticky")
	}

	if count != 0 {
		return true, nil
	}

	return false, nil
}

func (obj ObjectBase) SendEmailNotify() error {
	if setup := util.IsEmailSetup(); !setup {
		return nil
	}

	actor, _ := GetActorFromDB(obj.Actor)

	from := config.SiteEmail
	user := config.SiteEmailUsername
	pass := config.SiteEmailPassword
	to := config.SiteEmailNotifyTo
	posturl := config.Domain + "/" + actor.PreferredUsername + "/" + util.ShortURL(actor.Outbox, obj.Id)
	// If preview exists assume type image and use that
	// Else if no preview and Object.Attachment exists
	// check if video/audio to use correct element
	// Else if neither fall back to direct link
	var attachment string
	if obj.Attachment != nil {
		switch {
		case strings.Contains(obj.Attachment[0].MediaType, "video"):
			attachment = "<video controls style=\"max-width: 250px; max-height: 250px;\" src=\"" + obj.Attachment[0].Href + "\">Video is not supported.</video><br>" + obj.Attachment[0].Name + "<br>"
		case strings.Contains(obj.Attachment[0].MediaType, "audio"):
			attachment = "<audio controls style=\"max-width: 250px; max-height: 250px;\" src=\"" + obj.Attachment[0].Href + "\">Audio is not supported.</audio><br>" + obj.Attachment[0].Name + "<br>"
		case strings.Contains(obj.Attachment[0].MediaType, "image"):
			attachment = "<img src='" + obj.Preview.Href + "'><br>" + obj.Attachment[0].Name + "<br>"
		default:
			attachment = "Unknown attachment: <a href='" + obj.Attachment[0].Href + "'>" + obj.Attachment[0].Name + "</a>"
		}
	}
	//mime := ""
	//body := fmt.Sprintf("New post: %s\n\n--- Post ---\n%s\n", config.Domain+"/"+actor.PreferredUsername+"/"+util.ShortURL(actor.Outbox, obj.Id), obj.Content)
	mime := "MIME-version: 1.0;\nContent-Type: text/html; charset=\"UTF-8\";"
	body := fmt.Sprintf("<html><body>New post: <a href='%s'>%s</a><br><br>%s<br><br><b>%s %s</b><br>%s<br><pre>%s</pre></body></html>", posturl, posturl, attachment, obj.AttributedTo, obj.TripCode, obj.Name, obj.Content)

	msg := "From: " + config.InstanceName + " <" + from + ">\n" +
		"To: " + to + "\n" +
		"Subject: IB Post\n" +
		mime + "\n\n" + body
	err := smtp.SendMail(config.SiteEmailServer+":"+config.SiteEmailPort,
		smtp.PlainAuth(from, user, pass, config.SiteEmailServer),
		from, []string{to}, []byte(msg))

	return util.MakeError(err, "SendEmailNotify")
}
