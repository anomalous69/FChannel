package util

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"mime/multipart"
	"os"
	"path"
	"regexp"
	"runtime"
	"strings"

	"github.com/anomalous69/fchannel/config"
	"github.com/gabriel-vasile/mimetype"
)

var domainPattern string

func init() {
	domainPattern = fmt.Sprintf("%s/%%/", config.Domain)
}

func IsOnion(url string) bool {
	re := regexp.MustCompile(`(\.onion|\.loki|\.i2p)`)
	return re.MatchString(url)
}

func IsTorExit(ip string) bool {
	if len(config.TorExitList) == 0 {
		return false
	}
	exits, err := os.ReadFile(config.TorExitList)
	if err != nil {
		config.Log.Println("IsTorExit: Failed to read file \"" + config.TorExitList + ".")
		return false
	}

	isExit, err := regexp.Match(ip, exits)
	if err != nil {
		config.Log.Println("IsTorExit: Regex for IP (" + ip + ") failed.")
	}
	return isExit
}

func StripTransferProtocol(value string) string {
	re := regexp.MustCompile("(http://|https://)?(www.)?")
	value = re.ReplaceAllString(value, "")

	return value
}

func ShortURL(actorName string, url string) string {
	var reply string

	re := regexp.MustCompile(`.+\/`)
	actor := re.FindString(actorName)
	urlParts := strings.Split(url, "|")
	op := urlParts[0]

	if len(urlParts) > 1 {
		reply = urlParts[1]
	}

	re = regexp.MustCompile(`\w+$`)
	temp := re.ReplaceAllString(op, "")

	if temp == actor {
		id := LocalShort(op)

		re := regexp.MustCompile(`.+\/`)
		replyCheck := re.FindString(reply)

		if reply != "" && replyCheck == actor {
			id = id + "#" + LocalShort(reply)
		} else if reply != "" {
			id = id + "#" + RemoteShort(reply)
		}

		return id
	} else {
		id := RemoteShort(op)

		re := regexp.MustCompile(`.+\/`)
		replyCheck := re.FindString(reply)

		if reply != "" && replyCheck == actor {
			id = id + "#" + LocalShort(reply)
		} else if reply != "" {
			id = id + "#" + RemoteShort(reply)
		}

		return id
	}
}

func LocalShort(url string) string {
	re := regexp.MustCompile(`\w+$`)
	return re.FindString(StripTransferProtocol(url))
}

func RemoteShort(url string) string {
	re := regexp.MustCompile(`\w+$`)
	id := re.FindString(StripTransferProtocol(url))
	re = regexp.MustCompile(`.+/.+/`)
	actorurl := re.FindString(StripTransferProtocol(url))
	re = regexp.MustCompile(`/.+/`)
	actorname := re.FindString(actorurl)
	actorname = strings.Replace(actorname, "/", "", -1)

	return "f" + actorname + "-" + id
}

func ShortImg(url string) string {
	nURL := url
	re := regexp.MustCompile(`(\.\w+$)`)
	fileName := re.ReplaceAllString(url, "")

	if len(fileName) > 26 {
		re := regexp.MustCompile(`(^.{26})`)

		match := re.FindStringSubmatch(fileName)

		if len(match) > 0 {
			nURL = match[0]
		}

		re = regexp.MustCompile(`(\..+$)`)

		match = re.FindStringSubmatch(url)

		if len(match) > 0 {
			nURL = nURL + "(...)" + match[0]
		}
	}

	return nURL
}

func ConvertSize(size int64) string {
	var rValue string

	convert := float32(size) / 1024.0

	if convert > 1024 {
		convert = convert / 1024.0
		rValue = fmt.Sprintf("%.2f MiB", convert)
	} else {
		rValue = fmt.Sprintf("%.2f KiB", convert)
	}

	return rValue
}

// IsInStringArray looks for a string in a string array and returns true if it is found.
func IsInStringArray(haystack []string, needle string) bool {
	for _, e := range haystack {
		if e == needle {
			return true
		}
	}
	return false
}

// GetUniqueFilename will look for an available random filename in the /public/ directory.
func GetUniqueFilename(ext string) string {
	id := RandomID(8)
	file := "/public/" + id + "." + ext

	for {
		if _, err := os.Stat("." + file); err == nil {
			id = RandomID(8)
			file = "/public/" + id + "." + ext
		} else {
			return "/public/" + id + "." + ext
		}
	}
}

func HashMedia(media string) string {
	h := sha256.New()
	h.Write([]byte(media))
	return hex.EncodeToString(h.Sum(nil))
}

func HashBytes(media []byte) string {
	h := sha256.New()
	h.Write(media)
	return hex.EncodeToString(h.Sum(nil))
}

func EscapeString(text string) string {
	// TODO: not enough
	text = strings.Replace(text, "<", "&lt;", -1)
	return text
}

func CreateUniqueID(actor string) (string, error) {
	const maxAttempts = 10

	for attempt := 0; attempt < maxAttempts; attempt++ {
		newID := RandomID(8)
		query := "SELECT EXISTS(SELECT 1 FROM activitystream WHERE id >= $1 AND id < $2 LIMIT 1)"
		prefix := domainPattern + newID
		nextPrefix := domainPattern + newID[0:len(newID)-1] + string(newID[len(newID)-1]+1)

		var exists bool
		err := config.DB.QueryRow(query, prefix, nextPrefix).Scan(&exists)
		if err != nil {
			return "", MakeError(err, "CreateUniqueID")
		}

		if !exists {
			return newID, nil
		}

		config.Log.Printf("CreateUniqueID: ID collision detected for %s on attempt %d", newID, attempt+1)
	}

	return "", MakeError(errors.New("server failed to generate unique post id"), "CreateUniqueID")
}

func GetFileContentType(out multipart.File) (string, error) {
	buffer := make([]byte, 512)
	_, err := out.Read(buffer)

	if err != nil {
		return "", MakeError(err, "GetFileContentType")
	}

	out.Seek(0, 0)
	contentType := mimetype.Detect(buffer).String()

	// Handle APNG content types
	if contentType == "image/vnd.mozilla.apng" || contentType == "image/apng" {
		contentType = "image/png"
	}

	// Check for AVIF file with mif1 major brand
	if contentType == "image/heif" && len(buffer) >= 32 {
		if buffer[4] == 'f' && buffer[5] == 't' && buffer[6] == 'y' && buffer[7] == 'p' && // ftyp
			buffer[8] == 'm' && buffer[9] == 'i' && buffer[10] == 'f' && buffer[11] == '1' { // mif1
			// Look for 'avif' brand in compatible brands list
			for i := 16; i <= len(buffer)-4; i += 4 {
				if buffer[i] == 'a' && buffer[i+1] == 'v' && buffer[i+2] == 'i' && buffer[i+3] == 'f' {
					return "image/avif", nil
				}
			}
		}
	}

	return contentType, nil
}

func GetContentType(location string) string {
	elements := strings.Split(location, ";")

	if len(elements) > 0 {
		return elements[0]
	}

	return location
}

func CreatedNeededDirectories() error {
	if _, err := os.Stat("./public"); os.IsNotExist(err) {
		if err = os.Mkdir("./public", 0755); err != nil {
			return MakeError(err, "CreatedNeededDirectories")
		}
	}

	if _, err := os.Stat("./pem/board"); os.IsNotExist(err) {
		if err = os.MkdirAll("./pem/board", 0700); err != nil {
			return MakeError(err, "CreatedNeededDirectories")
		}
	}

	return nil
}

func LoadThemes() error {
	themes, err := os.ReadDir("./static/css/themes")

	if err != nil {
		MakeError(err, "LoadThemes")
	}

	for _, f := range themes {
		if e := path.Ext(f.Name()); e == ".css" {
			config.Themes = append(config.Themes, strings.TrimSuffix(f.Name(), e))
		}
	}

	return nil
}

func GetBoardAuth(board string) ([]string, error) {
	var auth []string
	var rows *sql.Rows
	var err error

	query := `select type from actorauth where board=$1`
	if rows, err = config.DB.Query(query, board); err != nil {
		return auth, MakeError(err, "GetBoardAuth")
	}

	defer rows.Close()
	for rows.Next() {
		var _type string
		if err := rows.Scan(&_type); err != nil {
			return auth, MakeError(err, "GetBoardAuth")
		}

		auth = append(auth, _type)
	}

	return auth, nil
}

func MakeError(err error, msg string) error {
	if err != nil {
		_, _, line, _ := runtime.Caller(1)
		s := fmt.Sprintf("%s:%d : %s", msg, line, err.Error())
		if config.Debug { // Catch and print pretty much all errors to log
			config.Log.Println(s)
		}
		return errors.New(s)
	}

	return nil
}

func SupportedMIMEType(mime string) bool {
	for _, e := range config.SupportedFiles {
		if e == mime {
			return true
		}
	}
	return false
}

func GetActorIdFromObjectId(objectId string) string {
	parts := strings.SplitN(objectId, "://", 2)
	if len(parts) != 2 {
		return ""
	}
	path := parts[1]
	pathParts := strings.SplitN(path, "/", 2)
	if len(pathParts) != 2 {
		return ""
	}
	boardParts := strings.SplitN(pathParts[1], "/", 2)
	if len(boardParts) < 1 {
		return ""
	}
	return parts[0] + "://" + pathParts[0] + "/" + boardParts[0]
}

// GetCountryName returns the country name for a given ISO 3166-1 alpha-2 country code
func GetCountryName(code string) string {
	code = strings.ToUpper(code)
	countries := map[string]string{
		// Official ISO 3166-1 alpha-2 country codes
		"AF": "Afghanistan",
		"AL": "Albania",
		"DZ": "Algeria",
		"AS": "American Samoa",
		"AD": "Andorra",
		"AO": "Angola",
		"AI": "Anguilla",
		"AQ": "Antarctica",
		"AG": "Antigua and Barbuda",
		"AR": "Argentina",
		"AM": "Armenia",
		"AW": "Aruba",
		"AU": "Australia",
		"AT": "Austria",
		"AZ": "Azerbaijan",
		"BS": "Bahamas",
		"BH": "Bahrain",
		"BD": "Bangladesh",
		"BB": "Barbados",
		"BY": "Belarus",
		"BE": "Belgium",
		"BZ": "Belize",
		"BJ": "Benin",
		"BM": "Bermuda",
		"BT": "Bhutan",
		"BO": "Bolivia",
		"BA": "Bosnia and Herzegovina",
		"BW": "Botswana",
		"BV": "Bouvet Island",
		"BR": "Brazil",
		"IO": "British Indian Ocean Territory",
		"BN": "Brunei Darussalam",
		"BG": "Bulgaria",
		"BF": "Burkina Faso",
		"BI": "Burundi",
		"KH": "Cambodia",
		"CM": "Cameroon",
		"CA": "Canada",
		"CV": "Cape Verde",
		"KY": "Cayman Islands",
		"CF": "Central African Republic",
		"TD": "Chad",
		"CL": "Chile",
		"CN": "China",
		"CX": "Christmas Island",
		"CC": "Cocos (Keeling) Islands",
		"CO": "Colombia",
		"KM": "Comoros",
		"CG": "Congo",
		"CD": "Congo, Democratic Republic of the",
		"CK": "Cook Islands",
		"CR": "Costa Rica",
		"CI": "Cote D'Ivoire",
		"HR": "Croatia",
		"CU": "Cuba",
		"CY": "Cyprus",
		"CZ": "Czech Republic",
		"DK": "Denmark",
		"DJ": "Djibouti",
		"DM": "Dominica",
		"DO": "Dominican Republic",
		"EC": "Ecuador",
		"EG": "Egypt",
		"SV": "El Salvador",
		"GQ": "Equatorial Guinea",
		"ER": "Eritrea",
		"EE": "Estonia",
		"ET": "Ethiopia",
		"FK": "Falkland Islands (Malvinas)",
		"FO": "Faroe Islands",
		"FJ": "Fiji",
		"FI": "Finland",
		"FR": "France",
		"GF": "French Guiana",
		"PF": "French Polynesia",
		"TF": "French Southern Territories",
		"GA": "Gabon",
		"GM": "Gambia",
		"GE": "Georgia",
		"DE": "Germany",
		"GH": "Ghana",
		"GI": "Gibraltar",
		"GR": "Greece",
		"GL": "Greenland",
		"GD": "Grenada",
		"GP": "Guadeloupe",
		"GU": "Guam",
		"GT": "Guatemala",
		"GN": "Guinea",
		"GW": "Guinea-Bissau",
		"GY": "Guyana",
		"HT": "Haiti",
		"HM": "Heard Island and McDonald Islands",
		"VA": "Holy See (Vatican City State)",
		"HN": "Honduras",
		"HK": "Hong Kong",
		"HU": "Hungary",
		"IS": "Iceland",
		"IN": "India",
		"ID": "Indonesia",
		"IR": "Iran",
		"IQ": "Iraq",
		"IE": "Ireland",
		"IL": "Israel",
		"IT": "Italy",
		"JM": "Jamaica",
		"JP": "Japan",
		"JO": "Jordan",
		"KZ": "Kazakhstan",
		"KE": "Kenya",
		"KI": "Kiribati",
		"KP": "North Korea",
		"KR": "South Korea",
		"KW": "Kuwait",
		"KG": "Kyrgyzstan",
		"LA": "Lao People's Democratic Republic",
		"LV": "Latvia",
		"LB": "Lebanon",
		"LS": "Lesotho",
		"LR": "Liberia",
		"LY": "Libya",
		"LI": "Liechtenstein",
		"LT": "Lithuania",
		"LU": "Luxembourg",
		"MO": "Macao",
		"MK": "North Macedonia",
		"MG": "Madagascar",
		"MW": "Malawi",
		"MY": "Malaysia",
		"MV": "Maldives",
		"ML": "Mali",
		"MT": "Malta",
		"MH": "Marshall Islands",
		"MQ": "Martinique",
		"MR": "Mauritania",
		"MU": "Mauritius",
		"YT": "Mayotte",
		"MX": "Mexico",
		"FM": "Micronesia",
		"MD": "Moldova",
		"MC": "Monaco",
		"MN": "Mongolia",
		"MS": "Montserrat",
		"MA": "Morocco",
		"MZ": "Mozambique",
		"MM": "Myanmar",
		"NA": "Namibia",
		"NR": "Nauru",
		"NP": "Nepal",
		"NL": "Netherlands",
		"NC": "New Caledonia",
		"NZ": "New Zealand",
		"NI": "Nicaragua",
		"NE": "Niger",
		"NG": "Nigeria",
		"NU": "Niue",
		"NF": "Norfolk Island",
		"MP": "Northern Mariana Islands",
		"NO": "Norway",
		"OM": "Oman",
		"PK": "Pakistan",
		"PW": "Palau",
		"PS": "Palestine",
		"PA": "Panama",
		"PG": "Papua New Guinea",
		"PY": "Paraguay",
		"PE": "Peru",
		"PH": "Philippines",
		"PN": "Pitcairn",
		"PL": "Poland",
		"PT": "Portugal",
		"PR": "Puerto Rico",
		"QA": "Qatar",
		"RE": "Reunion",
		"RO": "Romania",
		"RU": "Russian Federation",
		"RW": "Rwanda",
		"SH": "Saint Helena",
		"KN": "Saint Kitts and Nevis",
		"LC": "Saint Lucia",
		"PM": "Saint Pierre and Miquelon",
		"VC": "Saint Vincent and the Grenadines",
		"WS": "Samoa",
		"SM": "San Marino",
		"ST": "Sao Tome and Principe",
		"SA": "Saudi Arabia",
		"SN": "Senegal",
		"SC": "Seychelles",
		"SL": "Sierra Leone",
		"SG": "Singapore",
		"SK": "Slovakia",
		"SI": "Slovenia",
		"SB": "Solomon Islands",
		"SO": "Somalia",
		"ZA": "South Africa",
		"GS": "South Georgia and the South Sandwich Islands",
		"ES": "Spain",
		"LK": "Sri Lanka",
		"SD": "Sudan",
		"SR": "Suriname",
		"SJ": "Svalbard and Jan Mayen",
		"SZ": "Swaziland",
		"SE": "Sweden",
		"CH": "Switzerland",
		"SY": "Syrian Arab Republic",
		"TW": "Taiwan",
		"TJ": "Tajikistan",
		"TZ": "Tanzania",
		"TH": "Thailand",
		"TL": "Timor-Leste",
		"TG": "Togo",
		"TK": "Tokelau",
		"TO": "Tonga",
		"TT": "Trinidad and Tobago",
		"TN": "Tunisia",
		"TR": "Turkey",
		"TM": "Turkmenistan",
		"TC": "Turks and Caicos Islands",
		"TV": "Tuvalu",
		"UG": "Uganda",
		"UA": "Ukraine",
		"AE": "United Arab Emirates",
		"GB": "United Kingdom",
		"US": "United States",
		"UM": "United States Minor Outlying Islands",
		"UY": "Uruguay",
		"UZ": "Uzbekistan",
		"VU": "Vanuatu",
		"VE": "Venezuela (Bolivarian Republic of Venezuela)",
		"VN": "Vietnam",
		"VG": "Virgin Islands, British",
		"VI": "Virgin Islands, U.S.",
		"WF": "Wallis and Futuna",
		"EH": "Western Sahara",
		"YE": "Yemen",
		"ZM": "Zambia",
		"ZW": "Zimbabwe",
		"AX": "Åland Islands",
		"BL": "Saint Barthélemy",
		"BQ": "Bonaire, Sint Eustatius and Saba",
		"CW": "Curaçao",
		"GG": "Guernsey",
		"IM": "Isle of Man",
		"JE": "Jersey",
		"MF": "Saint Martin (French part)",
		"SS": "South Sudan",
		"SX": "Sint Maarten (Dutch part)",
		// Exceptional reservations
		"AC": "Ascension Island",
		"CP": "Clipperton Island",
		"CQ": "Island of Sark",
		"DG": "Diego Garcia",
		"EA": "Ceuta and Melilla",
		"EU": "European Union",
		"EZ": "Eurozone",
		"FX": "Metropolitan France",
		"IC": "Canary Islands",
		"SU": "Union of Soviet Socialist Republics (USSR)",
		"TA": "Tristan da Cunha",
		"UK": "United Kingdom",
		"UN": "United Nations",
		// Special codes
		"XI": "I2P (Invisible Internet Project)", // Used for I2P connections (not implemented)
		"XL": "Lokinet",                          // Used for Lokinet connections (not implemented)
		"XP": "Tor/Proxy",                        // Used for TOR/Other proxy connections
		"XX": "Unknown/Hidden",                   // Used when geolocation fails
	}

	if name, ok := countries[code]; ok {
		return name
	}
	config.Log.Println("GetCountryName: Unknown name for country code: ", code)
	return "Unknown Country (" + code + ")" // Fallback if MaxMind knows CC but we don't
}
