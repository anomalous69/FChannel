package util

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/anomalous69/fchannel/config"
	"github.com/anomalous69/fchannel/util/tripphrase"
	"github.com/gofiber/fiber/v2"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/simia-tech/crypt"
	oldcrypt "gitlab.com/nyarla/go-crypt"
	"golang.org/x/text/cases"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/language"
	"golang.org/x/text/transform"
)

const SaltTable = "" +
	"................................" +
	".............../0123456789ABCDEF" +
	"GABCDEFGHIJKLMNOPQRSTUVWXYZabcde" +
	"fabcdefghijklmnopqrstuvwxyz....." +
	"................................" +
	"................................" +
	"................................" +
	"................................"

func CreateNameTripCode(ctx *fiber.Ctx) (string, string, error) {
	input := ctx.FormValue("name")

	tripPhrase := regexp.MustCompile("###(.+)?")

	if tripPhrase.MatchString(input) {
		chunck := tripPhrase.FindString(input)
		chunck = strings.Replace(chunck, "###", "", 1)

		phrase, err := TripPhrase(chunck)

		return tripPhrase.ReplaceAllString(input, ""), phrase, MakeError(err, "CreateNameTripCode")
	}

	tripSecure := regexp.MustCompile("##(.+)?")

	if tripSecure.MatchString(input) {
		chunck := tripSecure.FindString(input)
		chunck = strings.Replace(chunck, "##", "", 1)
		ce := regexp.MustCompile(`(?i)Admin`)
		cemod := regexp.MustCompile(`(?i)Mod`)
		cejanitor := regexp.MustCompile(`(?i)Janitor`)
		admin := ce.MatchString(chunck)
		mod := cemod.MatchString(chunck)
		janitor := cejanitor.MatchString(chunck)
		board, modcred := GetPasswordFromSession(ctx)

		if hasAuth, modlevel := HasAuth(modcred, board); hasAuth {
			if chunck == "" { // If tripcode field is just ## then select correct modlevel for "user"
				return tripSecure.ReplaceAllString(input, ""), "#" + cases.Title(language.Und).String(modlevel), nil
			} // Allow admins to post as mods and janitors, allow mods to post as janitors
			// If a mod accidently posts with ##admin, or a janitor with ##admin or ##mod, fallback to their correct modlevel
			// Auth will be replaced with a proper Username & Password system soon, so this will work for now.
			if (admin) && (modlevel == "admin") {
				return tripSecure.ReplaceAllString(input, ""), "#Admin", nil
			} else if (mod || admin) && (modlevel == "admin" || modlevel == "mod") {
				return tripSecure.ReplaceAllString(input, ""), "#Moderator", nil
			} else if (janitor || mod || admin) && (modlevel == "admin" || modlevel == "janitor") {
				return tripSecure.ReplaceAllString(input, ""), "#Janitor", nil
			}
		}

		if chunck == "" {
			return tripSecure.ReplaceAllString(input, ""), "", nil
		}

		hash, err := TripCodeSecure(chunck)

		return tripSecure.ReplaceAllString(input, ""), "!!" + hash, MakeError(err, "CreateNameTripCode")
	}

	trip := regexp.MustCompile("#(.+)?")

	if trip.MatchString(input) {
		chunck := trip.FindString(input)
		chunck = strings.Replace(chunck, "#", "", 1)
		ce := regexp.MustCompile(`(?i)Admin`)
		cemod := regexp.MustCompile(`(?i)Mod`)
		cejanitor := regexp.MustCompile(`(?i)Janitor`)
		admin := ce.MatchString(chunck)
		mod := cemod.MatchString(chunck)
		janitor := cejanitor.MatchString(chunck)
		board, modcred := GetPasswordFromSession(ctx)

		if hasAuth, _ := HasAuth(modcred, board); hasAuth {
			if admin {
				return trip.ReplaceAllString(input, ""), "#Admin", nil
			} else if mod {
				return trip.ReplaceAllString(input, ""), "#Mod", nil
			} else if janitor {
				return trip.ReplaceAllString(input, ""), "#Janitor", nil
			}
		}

		hash := TripCode(chunck)
		return trip.ReplaceAllString(input, ""), "!" + hash, nil
	}

	return input, "", nil
}

func TripCode(pass string) string {
	var salt [2]rune

	if len(pass) > 8 {
		pass = pass[:8]
	}

	pass = TripCodeConvert(pass)
	s := []rune(pass + "H.")[1:3]

	for i, r := range s {
		salt[i] = rune(SaltTable[r%256])
	}

	enc := oldcrypt.Crypt(pass, string(salt[:]))

	return enc[len(enc)-10:]
}

func TripCodeConvert(str string) string {
	var s bytes.Buffer

	transform.NewWriter(&s, japanese.ShiftJIS.NewEncoder()).Write([]byte(str))
	re := strings.NewReplacer(
		"&", "&amp;",
		"\"", "&quot;",
		"<", "&lt;",
		">", "&gt;",
	)

	return re.Replace(s.String())
}

func TripCodeSecure(pass string) (string, error) {
	pass = TripCodeConvert(pass)
	enc, err := crypt.Crypt(pass, "$1$"+config.Salt)

	if err != nil {
		return "", MakeError(err, "TripCodeSecure")
	}

	return enc[len(enc)-10:], nil
}

func TripPhrase(pass string) (string, error) {
	pass = TripCodeConvert(pass)
	phrase, err := tripphrase.GeneratePhrase(pass)
	if err != nil {
		return "", MakeError(err, "TripPhrase")
	}

	return phrase, nil
}
