package activitypub

import (
	"crypto"
	crand "crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/anomalous69/fchannel/config"
	"github.com/anomalous69/fchannel/util"
	"github.com/gofiber/fiber/v2"
)

var ActorCache = make(map[string]Actor)

func (actor Actor) AddFollower(follower string) error {
	query := `insert into follower (id, follower) values ($1, $2)`
	_, err := config.DB.Exec(query, actor.Id, follower)
	return util.MakeError(err, "AddFollwer")
}

func (actor Actor) ActivitySign(signature string) (string, error) {
	if actor.PublicKey.Id == "" {
		actor, _ = GetActorFromDB(actor.Id)
	}

	var file string

	query := `select file from publicKeyPem where id=$1 `
	if err := config.DB.QueryRow(query, actor.PublicKey.Id).Scan(&file); err != nil {
		return "", util.MakeError(err, "ActivitySign")
	}

	file = strings.ReplaceAll(file, "public.pem", "private.pem")

	_, err := os.Stat(file)
	if err != nil {
		config.Log.Println(`\n Unable to locate private key. Now,
this means that you are now missing the proof that you are the
owner of the "` + actor.PreferredUsername + `" board. If you are the developer,
then your job is just as easy as generating a new keypair, but
if this board is live, then you'll also have to convince the other
owners to switch their public keys for you so that they will start
accepting your posts from your board from this site. Good luck ;)`)
		return "", util.MakeError(err, "ActivitySign")
	}

	var publickey []byte

	if publickey, err = os.ReadFile(file); err != nil {
		return "", util.MakeError(err, "ActivitySign")
	}

	block, _ := pem.Decode(publickey)
	pub, _ := x509.ParsePKCS1PrivateKey(block.Bytes)
	rng := crand.Reader
	hashed := sha256.New()
	hashed.Write([]byte(signature))
	cipher, _ := rsa.SignPKCS1v15(rng, pub, crypto.SHA256, hashed.Sum(nil))

	return base64.StdEncoding.EncodeToString(cipher), nil
}

func (actor Actor) ArchivePosts() error {
	// We don't want to archive posts from main or read only "meta" actors
	//  as this will push a bunch of threads from the boards they are following into the archive
	if actor.Id != "" && actor.Id != config.Domain && actor.PreferredUsername != "main" && !actor.HasOption(OptionReadOnly) {
		col, err := actor.GetAllArchive(165)

		if err != nil {
			return util.MakeError(err, "ArchivePosts")
		}

		for _, e := range col.OrderedItems {
			if e.Replies != nil {
				for _, k := range e.Replies.OrderedItems {
					if err := k.UpdateType("Archive"); err != nil {
						return util.MakeError(err, "ArchivePosts")
					}
				}
			}

			if err := e.UpdateType("Archive"); err != nil {
				return util.MakeError(err, "ArchivePosts")
			}
		}
	}

	return nil
}

func (actor Actor) AutoFollow() error {
	nActor, _ := GetActor(actor.Id)
	following, err := nActor.GetFollowing()

	if err != nil {
		return util.MakeError(err, "AutoFollow")
	}

	follower, err := nActor.GetFollower()

	if err != nil {
		return util.MakeError(err, "AutoFollow")
	}

	isFollowing := false

	for _, e := range follower {
		for _, k := range following {
			if e.Id == k.Id {
				isFollowing = true
			}
		}

		if !isFollowing && e.Id != config.Domain && e.Id != nActor.Id {
			followActivity, err := nActor.MakeFollowActivity(e.Id)

			if err != nil {
				return util.MakeError(err, "AutoFollow")
			}

			nActor, err := FingerActor(e.Id)

			if err != nil {
				return util.MakeError(err, "AutoFollow")
			}

			if nActor.Id != "" {
				followActivity.MakeRequestOutbox()
			}
		}
	}

	return nil
}

func (actor Actor) DeleteCache() error {
	query := `select id from cacheactivitystream where id in (select id from cacheactivitystream where actor=$1 and id in (select id from replies where inreplyto='')) `
	rows, err := config.DB.Query(query, actor.Id)

	if err != nil {
		return util.MakeError(err, "DeleteCache")
	}

	defer rows.Close()
	for rows.Next() {
		var obj ObjectBase
		if err := rows.Scan(&obj.Id); err != nil {
			return util.MakeError(err, "DeleteCache")
		}

		if err := obj.Delete(); err != nil {
			return util.MakeError(err, "DeleteCache")
		}
	}

	return nil
}

func (actor Actor) GetAllArchive(offset int) (Collection, error) {
	var nColl Collection
	var result []ObjectBase

	query := `select x.id, x.updated from (select id, updated from activitystream where actor=$1 and id in (select id from replies where inreplyto='') and type='Note' union select id, updated from activitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type='Note' union select id, updated from cacheactivitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type='Note') as x order by x.updated desc offset $2`
	rows, err := config.DB.Query(query, actor.Id, offset)

	if err != nil {
		return nColl, util.MakeError(err, "GetAllArchive")
	}

	defer rows.Close()
	for rows.Next() {
		var post ObjectBase

		if err := rows.Scan(&post.Id, &post.Updated); err != nil {
			return nColl, util.MakeError(err, "GetAllArchive")
		}

		post.Replies, err = post.GetReplies()

		result = append(result, post)
	}

	nColl.AtContext.Context = "https://www.w3.org/ns/activitystreams"

	nColl.OrderedItems = result

	return nColl, nil
}

func (actor Actor) GetAutoSubscribe() (bool, error) {
	var subscribed bool

	query := `select autosubscribe from actor where id=$1`
	if err := config.DB.QueryRow(query, actor.Id).Scan(&subscribed); err != nil {
		return false, util.MakeError(err, "GetAutoSubscribe")
	}

	return subscribed, nil
}

func (actor Actor) GetCatalogCollection() (Collection, error) {
	var nColl Collection
	var result []ObjectBase

	var err error
	var rows *sql.Rows

	query := `select x.id, x.name, x.content, x.type, x.published, x.updated, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (
		select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor=$1 and id in (select id from replies where inreplyto='') and type='Note' and id not in (select activity_id from sticky where actor_id=$1)
		union
		select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type='Note' and id not in (select activity_id from sticky where actor_id=$1)
		union
		select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from cacheactivitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type='Note' and id not in (select activity_id from sticky where actor_id=$1)
	) as x order by x.updated desc limit 165`

	if rows, err = config.DB.Query(query, actor.Id); err != nil {
		return nColl, util.MakeError(err, "GetCatalogCollection")
	}

	stickies, _ := actor.GetStickies()

	result = append(result, stickies.OrderedItems...)

	defer rows.Close()
	for rows.Next() {
		var post ObjectBase
		var actor Actor

		var attch ObjectBase

		var prev NestedObjectBase

		err = rows.Scan(&post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.Updated, &post.AttributedTo, &attch.Id, &prev.Id, &actor.Id, &post.TripCode, &post.Sensitive)

		if err != nil {
			return nColl, util.MakeError(err, "GetCatalogCollection")
		}

		post.Locked, _ = post.IsLocked()
		post.Actor = actor.Id

		post.Replies = &CollectionBase{}

		post.Replies.TotalItems, post.Replies.TotalImgs, err = post.GetRepliesCount()

		if err != nil {
			return nColl, util.MakeError(err, "GetCatalogCollection")
		}

		if attch.Id != "" {
			post.Attachment, err = attch.GetAttachment()
			if err != nil {
				return nColl, util.MakeError(err, "GetCatalogCollection")
			}
		}

		if prev.Id != "" {
			post.Preview, err = prev.GetPreview()
			if err != nil {
				return nColl, util.MakeError(err, "GetCatalogCollection")
			}
		}

		result = append(result, post)
	}

	nColl.AtContext.Context = "https://www.w3.org/ns/activitystreams"

	nColl.OrderedItems = result

	return nColl, nil
}

func (actor Actor) GetCollectionPage(page int) (Collection, error) {
	var nColl Collection
	var result []ObjectBase

	var err error
	var rows *sql.Rows

	query := `select count (x.id) over(), x.id, x.name, x.alias, x.content, x.type, x.published, x.updated, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (
		select id, name, alias, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor=$1 and id in (select id from replies where inreplyto='') and type='Note' and id not in (select activity_id from sticky where actor_id=$1)
		union
		select id, name, alias, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type='Note' and id not in (select activity_id from sticky where actor_id=$1)
		union
		select id, name, alias, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from cacheactivitystream where id not in (select activity_id from sticky where actor_id=$1) and actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type='Note'
	) as x order by x.updated desc limit $2 offset $3`

	limit := 15

	if page == 0 {
		stickies, _ := actor.GetStickies()
		limit = limit - stickies.TotalItems

		result = append(result, stickies.OrderedItems...)
	}

	offset := page * limit

	if rows, err = config.DB.Query(query, actor.Id, limit, offset); err != nil {
		return nColl, util.MakeError(err, "GetCollectionPage")
	}

	var count int
	defer rows.Close()
	for rows.Next() {
		var post ObjectBase
		var actor Actor

		var attch ObjectBase

		var prev NestedObjectBase

		err = rows.Scan(&count, &post.Id, &post.Name, &post.Alias, &post.Content, &post.Type, &post.Published, &post.Updated, &post.AttributedTo, &attch.Id, &prev.Id, &actor.Id, &post.TripCode, &post.Sensitive)

		if err != nil {
			return nColl, util.MakeError(err, "GetCollectionPage")
		}

		post.Locked, _ = post.IsLocked()
		post.Actor = actor.Id

		post.Replies, err = post.GetRepliesLimit(5)

		if err != nil {
			return nColl, util.MakeError(err, "GetCollectionPage")
		}

		if attch.Id != "" {
			post.Attachment, err = attch.GetAttachment()
			if err != nil {
				return nColl, util.MakeError(err, "GetCollectionPage")
			}
		}

		if err != nil {
			return nColl, util.MakeError(err, "GetCollectionPage")
		}

		if prev.Id != "" {
			post.Preview, err = prev.GetPreview()
			if err != nil {
				util.MakeError(err, "GetCollectionPage")
				continue
			}
		}

		result = append(result, post)
	}

	nColl.AtContext.Context = "https://www.w3.org/ns/activitystreams"

	nColl.TotalItems = count

	nColl.OrderedItems = result

	return nColl, nil
}

func (actor Actor) GetCollection() (Collection, error) {
	var nColl Collection
	var result []ObjectBase

	query := `select id, name, alias, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor=$1 and id in (select id from replies where inreplyto='') and type='Note' order by updated desc`
	rows, err := config.DB.Query(query, actor.Id)

	if err != nil {
		return nColl, util.MakeError(err, "GetCollection")
	}

	defer rows.Close()
	for rows.Next() {
		var post ObjectBase
		var actor Actor

		var attch ObjectBase

		var prev NestedObjectBase

		if err := rows.Scan(&post.Id, &post.Name, &post.Alias, &post.Content, &post.Type, &post.Published, &post.Updated, &post.AttributedTo, &attch.Id, &prev.Id, &actor.Id, &post.TripCode, &post.Sensitive); err != nil {
			return nColl, util.MakeError(err, "GetCollection")
		}

		post.Sticky, _ = post.IsSticky()
		post.Locked, _ = post.IsLocked()

		post.Actor = actor.Id

		post.Replies, err = post.GetReplies()

		if err != nil {
			return nColl, util.MakeError(err, "GetCollection")
		}

		if attch.Id != "" {
			post.Attachment, err = attch.GetAttachment()
			if err != nil {
				return nColl, util.MakeError(err, "GetCollection")
			}
		}

		if prev.Id != "" {
			post.Preview, err = prev.GetPreview()
			if err != nil {
				return nColl, util.MakeError(err, "GetCollection")
			}
		}

		result = append(result, post)
	}

	nColl.AtContext.Context = "https://www.w3.org/ns/activitystreams"

	nColl.OrderedItems = result

	return nColl, nil
}

func (actor Actor) GetCollectionType(nType string) (Collection, error) {
	var nColl Collection
	var result []ObjectBase

	query := `select x.id, x.name, x.alias, x.content, x.type, x.published, x.updated, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (
		select id, name, alias, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor=$1 and id in (select id from replies where inreplyto='') and type=$2 and id not in (select activity_id from sticky where actor_id=$1)
		union
		select id, name, alias, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type=$2 and id not in (select activity_id from sticky where actor_id=$1)
		union
		select id, name, alias, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from cacheactivitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type=$2 and id not in (select activity_id from sticky where actor_id=$1)
	) as x order by x.updated desc`
	rows, err := config.DB.Query(query, actor.Id, nType)
	if err != nil {
		return nColl, util.MakeError(err, "GetCollectionType")
	}

	if nType == "Note" {
		stickies, _ := actor.GetStickies()
		result = append(result, stickies.OrderedItems...)
	}

	defer rows.Close()
	for rows.Next() {
		var post ObjectBase
		var actor Actor

		var attch ObjectBase

		var prev NestedObjectBase

		if err := rows.Scan(&post.Id, &post.Name, &post.Alias, &post.Content, &post.Type, &post.Published, &post.Updated, &post.AttributedTo, &attch.Id, &prev.Id, &actor.Id, &post.TripCode, &post.Sensitive); err != nil {
			return nColl, util.MakeError(err, "GetCollectionType")
		}

		post.Locked, _ = post.IsLocked()

		post.Actor = actor.Id

		post.Replies = &CollectionBase{}

		post.Replies.TotalItems, post.Replies.TotalImgs, err = post.GetRepliesCount()
		if err != nil {
			return nColl, util.MakeError(err, "GetCollectionType")
		}

		if attch.Id != "" {
			post.Attachment, err = attch.GetAttachment()
			if err != nil {
				return nColl, util.MakeError(err, "GetCollectionType")
			}
		}

		if prev.Id != "" {
			post.Preview, err = prev.GetPreview()
			if err != nil {
				return nColl, util.MakeError(err, "GetCollectionType")
			}
		}

		result = append(result, post)
	}

	nColl.OrderedItems = result

	return nColl, nil
}

func (actor Actor) GetCollectionTypeLimit(nType string, limit int) (Collection, error) {
	var nColl Collection
	var result []ObjectBase

	query := `select x.id, x.name, x.alias, x.content, x.type, x.published, x.updated, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (
		select id, name, alias, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor=$1 and id in (select id from replies where inreplyto='') and type=$2
		union
		select id, name, alias, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type=$2
		union
		select id, name, alias, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from cacheactivitystream where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type=$2
	) as x order by x.updated desc limit $3`
	rows, err := config.DB.Query(query, actor.Id, nType, limit)

	if err != nil {
		return nColl, util.MakeError(err, "GetCollectionTypeLimit")
	}

	defer rows.Close()
	for rows.Next() {
		var post ObjectBase
		var actor Actor

		var attch ObjectBase

		var prev NestedObjectBase

		if err := rows.Scan(&post.Id, &post.Name, &post.Alias, &post.Content, &post.Type, &post.Published, &post.Updated, &post.AttributedTo, &attch.Id, &prev.Id, &actor.Id, &post.TripCode, &post.Sensitive); err != nil {
			return nColl, util.MakeError(err, "GetCollectionTypeLimit")
		}

		post.Actor = actor.Id

		post.Replies = &CollectionBase{}

		post.Replies.TotalItems, post.Replies.TotalImgs, err = post.GetRepliesCount()
		if err != nil {
			return nColl, util.MakeError(err, "GetCollectionTypeLimit")
		}

		if attch.Id != "" {
			post.Attachment, err = attch.GetAttachment()
			if err != nil {
				return nColl, util.MakeError(err, "GetCollectionTypeLimit")
			}
		}

		if prev.Id != "" {
			post.Preview, err = prev.GetPreview()
			if err != nil {
				return nColl, util.MakeError(err, "GetCollectionTypeLimit")
			}
		}

		result = append(result, post)
	}

	nColl.OrderedItems = result

	return nColl, nil
}

func (actor Actor) IsFollowing(following string) (bool, error) {
	query := `select following from following where id=$1 and following=$2`
	var followingId string
	err := config.DB.QueryRow(query, actor.Id, following).Scan(&followingId)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, util.MakeError(err, "IsFollowing")
	}
	return true, nil
}

func (actor Actor) GetFollower() ([]ObjectBase, error) {
	var followerCollection []ObjectBase

	query := `select follower from follower where id=$1`
	rows, err := config.DB.Query(query, actor.Id)

	if err != nil {
		return followerCollection, util.MakeError(err, "GetFollow")
	}

	defer rows.Close()
	for rows.Next() {
		var obj ObjectBase

		if err := rows.Scan(&obj.Id); err != nil {
			return followerCollection, util.MakeError(err, "GetFollow")
		}

		followerCollection = append(followerCollection, obj)
	}

	return followerCollection, nil
}

func (actor Actor) GetFollowing() ([]ObjectBase, error) {
	var followingCollection []ObjectBase

	query := `select following from following where id=$1`
	rows, err := config.DB.Query(query, actor.Id)

	if err != nil {
		return followingCollection, util.MakeError(err, "GetFollowing")
	}

	defer rows.Close()
	for rows.Next() {
		var obj ObjectBase

		if err := rows.Scan(&obj.Id); err != nil {
			return followingCollection, util.MakeError(err, "GetFollowing")
		}

		followingCollection = append(followingCollection, obj)
	}

	return followingCollection, nil
}

func (actor Actor) GetFollowFromName(name string) ([]string, error) {
	var followingActors []string

	activity := Activity{Id: actor.Following}
	follow, err := activity.GetCollection()
	if err != nil {
		return followingActors, util.MakeError(err, "GetFollowFromName")
	}

	re := regexp.MustCompile(`\w+?$`)

	for _, e := range follow.Items {
		if re.FindString(e.Id) == name {
			followingActors = append(followingActors, e.Id)
		}
	}

	return followingActors, nil
}

func (actor Actor) GetFollowingTotal() (int, error) {
	var following int

	query := `select count(following) from following where id=$1`
	if err := config.DB.QueryRow(query, actor.Id).Scan(&following); err != nil {
		return following, util.MakeError(err, "GetFollowingTotal")
	}

	return following, nil
}

func (actor Actor) GetFollowersTotal() (int, error) {
	var followers int

	query := `select count(follower) from follower where id=$1`
	if err := config.DB.QueryRow(query, actor.Id).Scan(&followers); err != nil {
		return followers, util.MakeError(err, "GetFollowersTotal")
	}

	return followers, nil
}

func (actor Actor) GetFollowersResp(ctx *fiber.Ctx) error {
	var following Collection
	var err error

	following.AtContext.Context = "https://www.w3.org/ns/activitystreams"
	following.Type = "Collection"
	following.TotalItems, err = actor.GetFollowersTotal()

	if err != nil {
		return util.MakeError(err, "GetFollowersResp")
	}

	following.Items, err = actor.GetFollower()

	if err != nil {
		return util.MakeError(err, "GetFollowersResp")
	}

	enc, _ := json.MarshalIndent(following, "", "\t")
	ctx.Response().Header.Set("Content-Type", config.ActivityStreams)
	_, err = ctx.Write(enc)

	return util.MakeError(err, "")
}

func (actor Actor) GetFollowingResp(ctx *fiber.Ctx) error {
	var following Collection
	var err error

	following.AtContext.Context = "https://www.w3.org/ns/activitystreams"
	following.Type = "Collection"
	following.TotalItems, err = actor.GetFollowingTotal()

	if err != nil {
		return util.MakeError(err, "GetFollowingResp")
	}

	following.Items, err = actor.GetFollowing()

	if err != nil {
		return util.MakeError(err, "GetFollowingResp")
	}

	enc, _ := json.MarshalIndent(following, "", "\t")
	ctx.Response().Header.Set("Content-Type", config.ActivityStreams)
	_, err = ctx.Write(enc)

	return util.MakeError(err, "GetFollowingResp")
}

func (actor Actor) GetImgTotal() (int, error) {
	var count int

	query := `select count(attachment) from activitystream where actor=$1 and id in (select id from replies where inreplyto='' and type='Note' )`
	if err := config.DB.QueryRow(query, actor.Id).Scan(&count); err != nil {
		return count, util.MakeError(err, "GetImgTotal")
	}

	return count, nil
}

func (actor Actor) GetInfoResp(ctx *fiber.Ctx) error {
	enc, _ := json.MarshalIndent(actor, "", "\t")
	ctx.Response().Header.Set("Content-Type", "application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\"")

	_, err := ctx.Write(enc)

	return util.MakeError(err, "GetInfoResp")
}

func (actor Actor) GetPostTotal() (int, error) {
	var count int

	query := `select count(id) from activitystream where actor=$1 and id in (select id from replies where inreplyto='' and type='Note')`
	if err := config.DB.QueryRow(query, actor.Id).Scan(&count); err != nil {
		return count, util.MakeError(err, "GetPostTotal")
	}

	return count, nil
}

func (actor Actor) GetOutbox(ctx *fiber.Ctx) error {
	var collection Collection

	c, err := actor.GetCollection()

	if err != nil {
		return util.MakeError(err, "GetOutbox")
	}

	collection.OrderedItems = c.OrderedItems
	collection.AtContext.Context = "https://www.w3.org/ns/activitystreams"
	collection.Actor = &actor

	collection.TotalItems, err = actor.GetPostTotal()

	if err != nil {
		return util.MakeError(err, "GetOutbox")
	}

	collection.TotalImgs, err = actor.GetImgTotal()

	if err != nil {
		return util.MakeError(err, "GetOutbox")
	}

	enc, _ := json.Marshal(collection)
	ctx.Response().Header.Set("Content-Type", config.ActivityStreams)
	_, err = ctx.Write(enc)

	return util.MakeError(err, "GetOutbox")
}

func (actor Actor) GetRecentPosts() ([]ObjectBase, error) {
	var collection []ObjectBase

	query := `select id, actor, content, published, attachment from activitystream where actor=$1 and type='Note' union select id, actor, content, published, attachment from cacheactivitystream where actor in (select follower from follower where id=$1) and type='Note' order by published desc limit 20`
	rows, err := config.DB.Query(query, actor.Id)

	if err != nil {
		return collection, util.MakeError(err, "GetRecentPosts")
	}

	defer rows.Close()
	for rows.Next() {
		var nObj ObjectBase
		var attachment ObjectBase

		err := rows.Scan(&nObj.Id, &nObj.Actor, &nObj.Content, &nObj.Published, &attachment.Id)

		if err != nil {
			return collection, util.MakeError(err, "GetRecentPosts")
		}

		isOP, _ := nObj.CheckIfOP()

		nObj.Attachment, _ = attachment.GetAttachment()

		if !isOP {
			var reply ObjectBase
			reply.Id = nObj.Id
			nObj.InReplyTo = append(nObj.InReplyTo, reply)
		}

		collection = append(collection, nObj)
	}

	return collection, nil
}

func (actor Actor) GetReported() ([]ObjectBase, error) {
	var nObj []ObjectBase

	query := `select id, count, reason from reported where board=$1`
	rows, err := config.DB.Query(query, actor.Id)

	if err != nil {
		return nObj, util.MakeError(err, "GetReported")
	}

	defer rows.Close()
	for rows.Next() {
		var obj ObjectBase

		err := rows.Scan(&obj.Id, &obj.Size, &obj.Content)

		if err != nil {
			return nObj, util.MakeError(err, "GetReported")
		}

		nObj = append(nObj, obj)
	}

	return nObj, nil
}

func (actor Actor) GetReportedTotal() (int, error) {
	var count int

	query := `select count(id) from reported where board=$1`
	if err := config.DB.QueryRow(query, actor.Id).Scan(&count); err != nil {
		return 0, util.MakeError(err, "GetReportedTotal")
	}

	return count, nil
}

func (actor Actor) HasValidation(ctx *fiber.Ctx) bool {
	id, _ := util.GetPasswordFromSession(ctx)

	if id == "" || (id != actor.Id && id != config.Domain) {
		return false
	}

	return true
}

func (actor Actor) IsAlreadyFollowing(follow string) (bool, error) {
	followers, err := actor.GetFollowing()

	if err != nil {
		return false, util.MakeError(err, "IsAlreadyFollowing")
	}

	for _, e := range followers {
		if e.Id == follow {
			return true, nil
		}
	}

	return false, nil
}

func (actor Actor) IsAlreadyFollower(follow string) (bool, error) {
	followers, err := actor.GetFollower()

	if err != nil {
		return false, util.MakeError(err, "IsAlreadyFollower")
	}

	for _, e := range followers {
		if e.Id == follow {
			return true, nil
		}
	}

	return false, nil
}

func (actor Actor) IsLocal() (bool, error) {
	actor, _ = GetActorFromDB(actor.Id)
	return actor.Id != "", nil
}

func (actor Actor) IsValid() (Actor, bool, error) {
	actor, err := FingerActor(actor.Id)
	return actor, actor.Id != "", util.MakeError(err, "IsValid")
}

func (actor Actor) ReportedResp(ctx *fiber.Ctx) error {
	var err error

	auth := ctx.Get("Authorization")
	verification := strings.Split(auth, " ")

	if len(verification) < 2 {
		ctx.Response().Header.SetStatusCode(http.StatusBadRequest)
		_, err := ctx.Write([]byte(""))
		return util.MakeError(err, "GetReported")
	}

	if hasAuth, _ := util.HasAuth(verification[1], actor.Id); !hasAuth {
		ctx.Response().Header.SetStatusCode(http.StatusBadRequest)
		_, err := ctx.Write([]byte(""))
		return util.MakeError(err, "GetReported")
	}

	actor, err = GetActorFromDB(actor.Id)

	if err != nil {
		return util.MakeError(err, "GetReported")
	}

	var following Collection

	following.AtContext.Context = "https://www.w3.org/ns/activitystreams"
	following.Type = "Collection"
	following.TotalItems, err = actor.GetReportedTotal()

	if err != nil {
		return util.MakeError(err, "GetReported")
	}

	following.Items, err = actor.GetReported()

	if err != nil {
		return util.MakeError(err, "GetReported")
	}

	enc, err := json.MarshalIndent(following, "", "\t")

	if err != nil {
		return util.MakeError(err, "GetReported")
	}

	ctx.Response().Header.Set("Content-Type", config.ActivityStreams)
	_, err = ctx.Write(enc)

	return util.MakeError(err, "GetReported")
}

func (actor Actor) SetAutoSubscribe() error {
	current, err := actor.GetAutoSubscribe()

	if err != nil {
		return util.MakeError(err, "SetAutoSubscribe")
	}

	query := `update actor set autosubscribe=$1 where id=$2`
	_, err = config.DB.Exec(query, !current, actor.Id)

	return util.MakeError(err, "SetAutoSubscribe")
}

func (actor Actor) SendToFollowers(activity Activity) error {
	followers, err := actor.GetFollower()

	if err != nil {
		return util.MakeError(err, "SendToFollowers")
	}

	var cc []string

	for _, e := range followers {
		var isTo = false

		for _, k := range activity.To {
			if e.Id != k {
				isTo = true
			}
		}

		if !isTo {
			cc = append(cc, e.Id)
		}
	}

	activity.To = make([]string, 0)
	activity.Cc = cc

	err = activity.MakeRequestInbox()

	return util.MakeError(err, "SendToFollowers")
}

func (actor Actor) UnArchiveLast() error {
	col, err := actor.GetCollectionTypeLimit("Archive", 1)

	if err != nil {
		return util.MakeError(err, "UnArchiveLast")
	}

	for _, e := range col.OrderedItems {
		for _, k := range e.Replies.OrderedItems {
			if err := k.UpdateType("Note"); err != nil {
				return util.MakeError(err, "UnArchiveLast")
			}
		}

		if err := e.UpdateType("Note"); err != nil {
			return util.MakeError(err, "UnArchiveLast")
		}
	}

	return nil
}

func (actor Actor) Verify(signature string, verify string) error {
	sig, _ := base64.StdEncoding.DecodeString(signature)

	if actor.PublicKey.PublicKeyPem == "" {
		_actor, err := FingerActor(actor.Id)

		if err != nil {
			return util.MakeError(err, "Verify")
		}

		actor = _actor
	}

	block, _ := pem.Decode([]byte(actor.PublicKey.PublicKeyPem))

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)

	if err != nil {
		return util.MakeError(err, "Verify")
	}

	hashed := sha256.New()
	hashed.Write([]byte(verify))

	return rsa.VerifyPKCS1v15(pub.(*rsa.PublicKey), crypto.SHA256, hashed.Sum(nil), sig)
}

func (actor Actor) VerifyHeaderSignature(ctx *fiber.Ctx) bool {
	if actor.PublicKey == nil || actor.PublicKey.Id == "" {
		return false
	}

	var sig string
	var path string
	var host string
	var date string
	var method string
	var digest string
	var contentLength string

	s := ParseHeaderSignature(ctx.Get("Signature"))

	for i, e := range s.Headers {
		var nl string
		if i < len(s.Headers)-1 {
			nl = "\n"
		}

		switch e {
		case "(request-target)":
			method = strings.ToLower(ctx.Method())
			path = ctx.Path()
			sig += "(request-target): " + method + " " + path + "" + nl
		case "host":
			host = ctx.Hostname()
			sig += "host: " + host + "" + nl
		case "date":
			date = ctx.Get("date")
			sig += "date: " + date + "" + nl
		case "digest":
			digest = ctx.Get("digest")
			sig += "digest: " + digest + "" + nl
		case "content-length":
			contentLength = ctx.Get("content-length")
			sig += "content-length: " + contentLength + "" + nl
		}
	}

	if s.KeyId != actor.PublicKey.Id {
		return false
	}

	t, _ := time.Parse(time.RFC1123, date)

	if time.Now().UTC().Sub(t).Seconds() > 75 {
		return false
	}

	if actor.Verify(s.Signature, sig) != nil {
		return false
	}

	return true
}

func (actor Actor) WriteCache() error {
	actor, err := FingerActor(actor.Id)

	if err != nil {
		return util.MakeError(err, "WriteCache")
	}

	reqActivity := Activity{Id: actor.Outbox}
	collection, err := reqActivity.GetCollection()

	if err != nil {
		return util.MakeError(err, "WriteCache")
	}

	for _, e := range collection.OrderedItems {
		if _, err := e.WriteCache(); err != nil {
			return util.MakeError(err, "WriteCache")
		}
	}

	return nil
}

func (actor Actor) MakeFollowActivity(follow string) (Activity, error) {
	var followActivity Activity
	var err error

	followActivity.AtContext.Context = "https://www.w3.org/ns/activitystreams"
	followActivity.Type = "Follow"

	var obj ObjectBase
	var nactor Actor

	if actor.Id == config.Domain {
		nactor, err = GetActorFromDB(actor.Id)
	} else {
		nactor, err = FingerActor(actor.Id)
	}

	if err != nil {
		return followActivity, util.MakeError(err, "MakeFollowActivity")
	}

	followActivity.Actor = &nactor
	followActivity.Object = obj

	followActivity.Object.Actor = follow
	followActivity.To = append(followActivity.To, follow)

	return followActivity, nil
}

func (actor Actor) WantToServePage(page int) (Collection, error) {
	var collection Collection
	var err error

	if page > config.PostCountPerPage && !actor.HasOption(OptionReadOnly) {
		return collection, errors.New("above page limit")
	}

	if collection, err = actor.GetCollectionPage(page); err != nil {
		return collection, util.MakeError(err, "WantToServePage")
	}

	collection.Actor = &actor

	return collection, nil
}

func (actor Actor) CreateVerification(verify util.Verify) error {
	var err error

	if verify.Code, err = util.CreateKey(50); err != nil {
		return util.MakeError(err, "CreateVerification")
	}

	if err := verify.Create(); err != nil {
		return util.MakeError(err, "CreateVerification")
	}

	verify.Board = actor.Id
	verify.Identifier = verify.Type

	if err := verify.CreateBoardMod(); err != nil {
		return util.MakeError(err, "CreateVerification")
	}

	return nil
}

func (actor Actor) DeleteVerification(verify util.Verify) error {
	query := `delete from boardaccess where code=$1`
	if _, err := config.DB.Exec(query, verify.Code); err != nil {
		return util.MakeError(err, "DeleteVerification")
	}

	var code string
	query = `select verificationcode from crossverification where code=$1`
	if err := config.DB.QueryRow(query, verify.Code).Scan(&code); err != nil {
		return util.MakeError(err, "DeleteVerification")
	}

	query = `delete from crossverification where code=$1`
	if _, err := config.DB.Exec(query, verify.Code); err != nil {
		return util.MakeError(err, "DeleteVerification")
	}

	query = `delete from verification where code=$1`
	if _, err := config.DB.Exec(query, code); err != nil {
		return util.MakeError(err, "DeleteVerification")
	}

	return nil
}

func (actor Actor) GetJanitors() ([]util.Verify, error) {
	var list []util.Verify

	query := `select identifier, code, board, type, label from boardaccess where board=$1 and type='janitor'`
	rows, err := config.DB.Query(query, actor.Id)

	if err != nil {
		return list, util.MakeError(err, "GetJanitors")
	}

	defer rows.Close()
	for rows.Next() {
		var verify util.Verify

		rows.Scan(&verify.Identifier, &verify.Code, &verify.Board, &verify.Type, &verify.Label)

		list = append(list, verify)
	}

	return list, nil
}

func (actor Actor) ProcessInboxCreate(activity Activity) error {
	if local, _ := actor.IsLocal(); local {
		if local, _ := activity.Actor.IsLocal(); !local {
			isFollowing, err := actor.IsFollowing(activity.Actor.Id)
			if err != nil {
				return util.MakeError(err, "ActorInbox")
			}
			if !isFollowing {
				return nil
			}

			// Attempt to fetch InReplyTo collections if missing locally
			if len(activity.Object.InReplyTo) > 0 {
				for _, inReplyTo := range activity.Object.InReplyTo {
					col, err := inReplyTo.GetCollectionLocal()
					if err != nil || len(col.OrderedItems) == 0 {
						// Check if following the inReplyTo actor/board
						inReplyToActorId := util.GetActorIdFromObjectId(inReplyTo.Id)
						isFollowingInReplyTo, err := actor.IsFollowing(inReplyToActorId)
						if err != nil {
							return util.MakeError(err, "ActorInbox: IsFollowing(inReplyTo)")
						}
						if isFollowingInReplyTo {
							remoteCol, fetchErr := Activity{Id: inReplyTo.Id}.GetCollection()
							if fetchErr != nil || len(remoteCol.OrderedItems) == 0 {
								return util.MakeError(fetchErr, "ActorInbox: failed to fetch InReplyTo collection")
							}
							for _, post := range remoteCol.OrderedItems {
								if _, err := post.WriteCache(); err != nil {
									return util.MakeError(err, "ActorInbox: failed to store remote InReplyTo post")
								}
							}
						}
					}
				}
			}

			reqActivity := Activity{Id: activity.Object.Id}
			col, err := reqActivity.GetCollection()
			if err != nil {
				return util.MakeError(err, "ActorInbox")
			}

			if len(col.OrderedItems) < 1 {
				return util.MakeError(errors.New("Object does not exist"), "ActorInbox")
			}

			if len(activity.Object.InReplyTo) > 0 {
				if locked, _ := activity.Object.InReplyTo[0].IsLocked(); locked {
					return util.MakeError(errors.New("Object locked"), "ActorInbox")
				}
			}

			if wantToCache, err := activity.Object.WantToCache(actor); !wantToCache {
				return util.MakeError(err, "ActorInbox")
			}

			if col, _ := activity.Object.GetCollectionLocal(); len(col.OrderedItems) != 0 {
				return nil
			}

			if _, err := activity.Object.WriteCache(); err != nil {
				return util.MakeError(err, "ActorInbox")
			}

			if err := actor.ArchivePosts(); err != nil {
				return util.MakeError(err, "ActorInbox")
			}

			activity.Object.Actor = actor.Id

			go func(obj ObjectBase) {
				err := obj.SendEmailNotify()

				if err != nil {
					config.Log.Println(err)
				}
			}(activity.Object)
		}
	}

	return nil
}

func (actor Actor) GetStickies() (Collection, error) {
	var nColl Collection
	var result []ObjectBase

	query := `
select count
(x.id) over(), x.id, x.name, x.alias, x.content, x.type, x.published, x.updated, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive
	from
	 (select id, name, alias, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream
		where actor=$1 and id in (select id from replies where inreplyto='') and type='Note' and id in (select activity_id from sticky where actor_id=$1)
	union
		select id, name, alias, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from activitystream
		where actor in (select following from following where id=$1) and id in (select id from replies where inreplyto='') and type='Note' and id in (select activity_id from sticky where actor_id=$1)
	union
		select id, name, alias, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive from cacheactivitystream where actor in (select following from following where id=$1)
		and id in (select id from replies where inreplyto='') and type='Note' and id in (select activity_id from sticky where actor_id=$1)
) as x order by x.updated desc limit 15`

	rows, err := config.DB.Query(query, actor.Id)

	if err != nil {
		return nColl, util.MakeError(err, "GetStickies")
	}

	var count int
	defer rows.Close()
	for rows.Next() {
		var post ObjectBase
		var actor Actor

		var attch ObjectBase

		var prev NestedObjectBase

		err = rows.Scan(&count, &post.Id, &post.Name, &post.Alias, &post.Content, &post.Type, &post.Published, &post.Updated, &post.AttributedTo, &attch.Id, &prev.Id, &actor.Id, &post.TripCode, &post.Sensitive)

		if err != nil {
			return nColl, util.MakeError(err, "GetStickies")
		}

		post.Sticky = true
		post.Locked, _ = post.IsLocked()
		post.Actor = actor.Id

		post.Replies, err = post.GetRepliesLimit(5)
		if err != nil {
			return nColl, err
		}

		if attch.Id != "" {
			post.Attachment, err = attch.GetAttachment()
			if err != nil {
				return nColl, util.MakeError(err, "GetStickies")
			}
		}

		if prev.Id != "" {
			post.Preview, err = prev.GetPreview()
			if err != nil {
				return nColl, err
			}
		}

		result = append(result, post)
	}

	nColl.TotalItems = count
	nColl.OrderedItems = result

	return nColl, nil
}

func (actor Actor) GetRecentThreads() (Collection, error) {
	//TODO: Convert ID and Board to local
	var nColl Collection
	var result []ObjectBase

	var err error
	var rows *sql.Rows

	// Selects 6 most recently created local/remote threads
	// Will exclude "hidden" boards when main actor used (only display local boards followed by main actor, and remote boards where the local actor is followed by the main actor)
	query := `select x.id, x.name, x.content, x.type, x.published, x.updated, x.attributedto, x.attachment, x.preview, x.actor, x.tripcode, x.sensitive from (
		select a.id, a.name, a.content, a.type, a.published, a.updated, a.attributedto, a.attachment, a.preview,
			actor.name as actor, a.tripcode, a.sensitive
		from activitystream a
		join actor on actor.id = a.actor
		where a.actor in (select following from following where id=$1)
			and a.id in (select id from replies where inreplyto='')
			and a.type='Note'
			and a.id not in (select activity_id from sticky where actor_id=$1)
	union
		select id, name, content, type, published, updated, attributedto, attachment, preview, actor, tripcode, sensitive
		from cacheactivitystream
		where actor in (select following from following where id in (select following from following where id=$1))
			and id in (select id from replies where inreplyto='')
			and type='Note'
			and id not in (select activity_id from sticky where actor_id=$1)
	) as x order by x.published desc limit 6`

	if rows, err = config.DB.Query(query, actor.Id); err != nil {
		return nColl, util.MakeError(err, "GetRecentThreads")
	}

	defer rows.Close()
	for rows.Next() {
		var post ObjectBase
		var actor Actor

		var attch ObjectBase

		var prev NestedObjectBase

		err = rows.Scan(&post.Id, &post.Name, &post.Content, &post.Type, &post.Published, &post.Updated, &post.AttributedTo, &attch.Id, &prev.Id, &actor.Id, &post.TripCode, &post.Sensitive)

		if err != nil {
			return nColl, util.MakeError(err, "GetRecentThreads")
		}

		post.Locked, _ = post.IsLocked()
		post.Actor = actor.Id

		post.Replies = &CollectionBase{}

		post.Replies.TotalItems, post.Replies.TotalImgs, err = post.GetRepliesCount()

		if err != nil {
			return nColl, util.MakeError(err, "GetRecentThreads")
		}

		if attch.Id != "" {
			post.Attachment, err = attch.GetAttachment()
			if err != nil {
				return nColl, util.MakeError(err, "GetRecentThreads")
			}
		}

		if prev.Id != "" {
			post.Preview, err = prev.GetPreview()
			if err != nil {
				return nColl, util.MakeError(err, "GetRecentThreads")
			}
		}

		result = append(result, post)
	}

	nColl.AtContext.Context = "https://www.w3.org/ns/activitystreams"

	nColl.OrderedItems = result

	return nColl, nil
}

func (actor Actor) GetBoardType() string {
	//TODO: unused!
	var boardtype string

	query := `select boardtype from actor where id=$1`
	if err := config.DB.QueryRow(query, actor.Id).Scan(&boardtype); err != nil {
		boardtype = "image"
	}

	return boardtype
}

const (
	OptionID        = 1 << 0 // 1
	OptionFlag      = 1 << 1 // 2
	OptionTripcode  = 1 << 2 // 4
	OptionAnonymous = 1 << 3 // 8
	OptionReadOnly  = 1 << 4 // 16
)

// HasOption returns true if the actor's OptionsMask contains the given option bit(s)
func (a *Actor) HasOption(option int) bool {
	return a.OptionsMask&option != 0
}
