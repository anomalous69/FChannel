package config

import (
	"bufio"
	"database/sql"
	"errors"
	"log"
	"os"
	"strconv"
	"strings"
)

var Port = ":" + GetConfigValue("instanceport", "3000")
var TP = GetConfigValue("instancetp", "")
var Domain = TP + "" + GetConfigValue("instance", "")
var InstanceName = GetConfigValue("instancename", "")
var InstanceSummary = GetConfigValue("instancesummary", "")
var SiteEmail = GetConfigValue("emailaddress", "")
var SiteEmailUsername = GetConfigValue("emailuser", "")
var SiteEmailPassword = GetConfigValue("emailpass", "")
var SiteEmailServer = GetConfigValue("emailserver", "")
var SiteEmailPort = GetConfigValue("emailport", "")
var SiteEmailNotifyTo = GetConfigValue("emailnotify", "")
var NtfyURL = GetConfigValue("ntfyurl", "")
var NtfyAuth = GetConfigValue("ntfyauth", "")
var TorProxy = GetConfigValue("torproxy", "")
var Salt = GetConfigValue("instancesalt", "")
var DBHost = GetConfigValue("dbhost", "localhost")
var DBPort, _ = strconv.Atoi(GetConfigValue("dbport", "5432"))
var DBUser = GetConfigValue("dbuser", "postgres")
var DBPassword = GetConfigValue("dbpass", "password")
var DBName = GetConfigValue("dbname", "server")
var CookieKey = GetConfigValue("cookiekey", "")
var ActivityStreams = "application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\""
var AuthReq = []string{"captcha", "email", "passphrase"}
var PostCountPerPage = 10
var SupportedFiles = []string{"image/avif", "image/gif", "image/jpeg", "image/jxl", "image/png", "image/webp", "video/mp4", "video/ogg", "video/webm", "audio/mpeg", "audio/ogg", "audio/wav", "audio/wave", "audio/x-wav", "application/x-shockwave-flash"}
var Log = log.New(os.Stdout, "", log.Ltime)
var MediaHashs = make(map[string]string)
var Key = GetConfigValue("modkey", "")
var MinPostDelete = GetConfigValue("minpostdelete", "60")
var MaxPostDelete = GetConfigValue("maxpostdelete", "1800")

// TODO: this is bad but I don't feel like doing a new config system yet, and I can't into computers
var MaxAttachmentSize, _ = strconv.Atoi(GetConfigValue("maxattachsize", "7340032"))
var MaxMindDB = GetConfigValue("maxminddb", "")
var TorExitList = GetConfigValue("torexitlist", "")
var ProxyHeader = GetConfigValue("proxyheader", "")
var CaptchaFont = GetConfigValue("captchafont", "") // TODO: should probably check user not passing anything weird to exec.Command
var Themes []string
var DB *sql.DB
var Version string
var Debug = true //TODO: read this from config file

// TODO: Change this to some other config format like YAML
// to save into a struct and only read once
func GetConfigValue(value string, ifnone string) string {
	file, err := os.Open("fchan.cfg")

	if err != nil {
		//TODO: Really poor temporary detection
		//      Remove this sometime in the future
		//      This could probably be moved automatically
		if errors.Is(err, os.ErrNotExist) {
			if _, err := os.Stat("config/config-init"); err == nil {
				Log.Println("!!!!!!! ATTENTION !!!!!!!")
				Log.Println("!!!!!!!  ACHTUNG !!!!!!!!")
				Log.Println("Config file 'fchan.cfg' does not exist!")
				Log.Println("Detected old config file, please move 'config/config-init' to 'fchan.cfg'")
				Log.Println("!!!!!!!!!!!!!!!!")
				Log.Println("!!!!!!!!!!!!!!!!")
				os.Exit(2)
			}
		}
		Log.Println(err)
		return ifnone
	}

	defer file.Close()

	lines := bufio.NewScanner(file)

	for lines.Scan() {
		line := strings.SplitN(lines.Text(), ":", 2)
		if line[0] == value {
			return line[1]
		}
	}

	return ifnone
}
