package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"github.com/jhillyerd/enmime"
	"github.com/spf13/viper"
	"hash/crc64"
	"html"
	"io/ioutil"
	"math/rand"
	"net/http"
	_ "net/mail"
	"net/smtp"
	"net/url"
	"os"
	"path/filepath"
	"perso.tld/CountablyMany/syncer"
	"strconv"
	"strings"
	"time"
)

var separ string
var ResetCacheMTime int64

func HookAuth(r http.ResponseWriter, q *http.Request) bool {
	lhash := viper.GetString("LoginHash")
	if lhash == "" {
		return false
	}
	if "Basic "+lhash != q.Header.Get("Authorization") {
		r.Header().Set("WWW-Authenticate", "Basic realm=restricted")
		r.WriteHeader(401)
		return false
	}
	return true
}

func HdlRes(r http.ResponseWriter, q *http.Request) {
	if !HookAuth(r, q) {
		return
	}
	if q.FormValue("q") == "js" {
		http.ServeFile(r, q, "script.js")
	} else if q.FormValue("q") == "css" {
		http.ServeFile(r, q, "style.css")
	} else {
		http.ServeFile(r, q, "index.html")
	}
}

var SyncerConfig syncer.Config
var SyncerIes syncer.IndexEntries

func HdlCmd(r http.ResponseWriter, q *http.Request) {
	if !HookAuth(r, q) {
		return
	}
	query := q.FormValue("q")

	querys := strings.Split(query, "##")
	if len(querys) > 1 {
		subject := querys[0]
		movedest := querys[1]
		subjectspl := strings.Split(subject, "/")
		fnam := SyncerConfig.Path + separ + subjectspl[0] + separ + subjectspl[1] + separ + "moves" + separ + subjectspl[2]
		cnt := []byte(movedest)
		err := ioutil.WriteFile(fnam, cnt, 0660)
		fmt.Fprint(r, "wrote "+string(cnt)+" in "+fnam+" ", err)
		return
	}

	SyncerIes = SyncerConfig.ReadIndexEntries()
	outstr := SyncerIes.ListMessagesHTML(query, SyncerConfig.Path)

	if HandleETag(r, q, ETagS(outstr)) {
		return
	}

	fmt.Fprintf(r, outstr)
}

func GetMessageFile(r http.ResponseWriter, q *http.Request) (*os.File, string) {
	id := strings.ReplaceAll(q.FormValue("id"), "..", "")
	fname := SyncerConfig.Path + separ + id
	file, err2 := os.Open(fname)
	if err2 != nil {
		fmt.Fprint(r, "Can't open "+fname)
		return nil, ""
	}
	return file, fname
}

func HdlSource(r http.ResponseWriter, q *http.Request) {
	if !HookAuth(r, q) {
		return
	}

	SyncerIes = SyncerConfig.ReadIndexEntries()
	r.Header().Set("Content-type", "text/plain")
	_, fname := GetMessageFile(r, q)
	http.ServeFile(r, q, fname)
}

func ETagF(fnam string) string {
	ResetCacheL, err := os.Lstat(".reset_cache")
	if err != nil {
		ResetCacheMTime = 0
	} else {
		ResetCacheMTime = ResetCacheL.ModTime().Unix()
	}
	lst, err := os.Lstat(fnam)
	if err != nil {
		fmt.Println("ETagF: failed Lstat ", err)
		s := strconv.Itoa(int(rand.Uint64()))
		return s
	}
	mtime := lst.ModTime().Unix()
	return fmt.Sprintf("%x%x", ResetCacheMTime, mtime)
}

func ETagS(str string) string {
	ResetCacheL, err := os.Lstat(".reset_cache")
	if err != nil {
		ResetCacheMTime = 0
	} else {
		ResetCacheMTime = ResetCacheL.ModTime().Unix()
	}
	return fmt.Sprintf("%x%x", ResetCacheMTime, crc64.Checksum([]byte(str), crc64.MakeTable(crc64.ECMA)))
}

func HandleETag(r http.ResponseWriter, q *http.Request, etag string) bool {
	r.Header().Set("ETag", etag)
	if q.Header.Get("If-None-Match") == etag {
		r.WriteHeader(304)
		return true
	}
	return false
}

func HdlRead(r http.ResponseWriter, q *http.Request) {
	if !HookAuth(r, q) {
		return
	}

	SyncerIes = SyncerConfig.ReadIndexEntries()
	id := q.FormValue("id")
	file, fname := GetMessageFile(r, q)
	if HandleETag(r, q, ETagF(fname)) {
		return
	}
	mail, err2 := enmime.ReadEnvelope(bufio.NewReader(file))
	if err2 != nil {
		fmt.Fprint(r, "Can't parse mail id "+id)
		return
	}
	fmt.Fprint(r, "<div id=headers><table><tr><td><b>From</b><td>"+mail.GetHeader("From")+
		"<tr><td><b>To</b><td>"+html.EscapeString(mail.GetHeader("To")+", "+mail.GetHeader("Cc"))+
		"<tr><td><b>Subject</b><td>"+html.EscapeString(mail.GetHeader("Subject"))+
		"<tr><td><b>Date</b><td>"+html.EscapeString(mail.GetHeader("Date"))+"</table>")
	_ = fname
	fmt.Fprint(r, "<div id=attachments><a href=/source?id="+url.QueryEscape(id)+" target=_new>src</a>" /* ["+fname+"]" */ +"<br>")
	for _, att := range append(mail.Attachments, mail.Inlines...) {
		url := "/attachget?id=" + url.QueryEscape(id) + "&cid=" + url.QueryEscape(att.FileName)
		url1 := url + "&mode=inline"
		url2 := url + "&mode=attach"
		fmt.Fprint(r, "<a href="+url1+" target=_new>"+att.FileName+"</a> ("+att.ContentType+") <a href="+url2+">[dl]</a><br>")
	}

	fmt.Fprint(r, "</div></div><div id=mailbody>")
	htmlmail := string(mail.HTML)
	if htmlmail == "" {
		htmlmail = string(mail.Text)
		htmlmail = strings.ReplaceAll(htmlmail, "\n", "<br>")
	}
	fmt.Fprint(r, htmlmail+"</div>")

}

func HdlReplytemplate(r http.ResponseWriter, q *http.Request) {
	if !HookAuth(r, q) {
		return
	}

	SyncerIes = SyncerConfig.ReadIndexEntries()
	id := q.FormValue("id")
	fwdMode:=(q.FormValue("mode")=="f")
	file, fname := GetMessageFile(r, q)
	_ = fname
	/*	if HandleETag(r, q, ETagF(fname)) {
		return
	} */
	mail, err2 := enmime.ReadEnvelope(bufio.NewReader(file))
	if err2 != nil {
		fmt.Fprint(r, "Can't parse mail id "+id+" ", err2)
		return
	}
	replyto:=""
	subjectre:=""
	if !fwdMode {
		replyto = mail.GetHeader("From")
		if mail.GetHeader("Reply-to") != "" {
			replyto = mail.GetHeader("Reply-to")
		}
		if strings.Index(replyto, "<") >= 0 {
			replyto = strings.Split(replyto, "<")[1]
			replyto = replyto[:len(replyto)-1]
		}
		subjectre = "Re: " + mail.GetHeader("Subject")
		if strings.Index(mail.GetHeader("Subject"), "Re:") >= 0 || strings.Index(mail.GetHeader("Subject"), "re:") >= 0 {
			subjectre = mail.GetHeader("Subject")
		}
	} else {
		subjectre = "Fwd: " + mail.GetHeader("Subject")
	}
	mailtxt := mail.Text
	mailtxt = "> " + strings.ReplaceAll(mailtxt, "\n", "\n> ")
	replyidentity := "default"
	for identity, _ := range OutIdentities {
		outId := (OutIdentities[identity]).(map[string]interface{})
		fromaddr := outId["fromaddr"].(string)
		if strings.Index(mail.GetHeader("To")+mail.GetHeader("Cc"), fromaddr) >= 0 {
			replyidentity = identity
			break
		}
	}
	fmt.Fprint(r, replyidentity+"\r\n"+
		"To: "+replyto+"\r\n"+
		"Cc: \r\n"+
		"Subject: "+subjectre+"\r\n")
	if !fwdMode {
		fmt.Fprint(r,"In-reply-to: "+mail.GetHeader("Message-ID")+"\r\n")
	}
	fmt.Fprint(r,"References: "+mail.GetHeader("Message-ID")+" "+mail.GetHeader("References")+"\r\n"+
		"@endheaders\r\n"+
		"\r\n\r\n\r\n"+
		"--- Original message ---\r\n"+
		"From: "+mail.GetHeader("From")+"\r\n"+
		"Subject: "+mail.GetHeader("Subject")+"\r\n"+
		"Date: "+mail.GetHeader("Date")+"\r\n\r\n"+mailtxt)
	if fwdMode {
		fmt.Fprint(r,"\r\n@attachments "+id)
	}
}

func HdlAttachGet(r http.ResponseWriter, q *http.Request) {
	if !HookAuth(r, q) {
		return
	}

	SyncerIes = SyncerConfig.ReadIndexEntries()
	cid := q.FormValue("cid")
	mode := q.FormValue("mode")
	file, fname := GetMessageFile(r, q)
	if HandleETag(r, q, ETagF(fname)) {
		return
	}
	mail, err2 := enmime.ReadEnvelope(bufio.NewReader(file))
	if err2 != nil {
		fmt.Fprint(r, "Can't parse mail")
		return
	}
	for _, att := range append(mail.Attachments, mail.Inlines...) {
		if att.FileName == cid {
			r.Header().Set("Content-type", att.ContentType)
			if mode == "attach" {
				r.Header().Set("Content-disposition", "attachment;filename=\""+att.FileName+"\"")
			} else {
				r.Header().Set("Content-disposition", "inline")
			}
			fmt.Fprintf(r, "%s", att.Content)
			return
		}
	}
	fmt.Fprint(r, "CID not found in mail")
}

func HdlCompose(r http.ResponseWriter, q *http.Request) {
	if !HookAuth(r, q) {
		return
	}

}

func headerStr(header string, value string) (s string) {
	if value != "" {
		return header + ": " + value + "\r\n"
	} else {
		return ""
	}
}

func addAttach(r http.ResponseWriter, q *http.Request, suffix string, boundary string) string {
	mpf, mpfh, er := q.FormFile("attach" + suffix)
	if er != nil {
		return ""
	}
	d, _ := ioutil.ReadAll(mpf)
	return "\n--" + boundary + "\n" +
		"Content-disposition: attachment;filename=" + mpfh.Filename + "\n" +
		"Content-type: " + mpfh.Header.Get("Content-type") + "\n" +
		"Content-transfer-encoding: base64\n\n" +
		base64.StdEncoding.EncodeToString(d) + "\n"
}

func HdlSend(r http.ResponseWriter, q *http.Request) {
	if !HookAuth(r, q) {
		return
	}

	composeText := q.FormValue("compose")
	composeText = strings.ReplaceAll(composeText, "\r", "")

	var identity string
	fmt.Sscanf(composeText, "%s\n", &identity)

	boundary := "b" + fmt.Sprintf("%x", rand.Uint64())
	endheaders := "Date: " + time.Now().Format(time.RFC1123Z) + "\n" +
		"Content-transfer-encoding: 8bit\n" +
		"Content-type: multipart/alternative;boundary=" + boundary + "\n" +
		"MIME-Version: 1.0\n\n" +
		"--" + boundary + "\n" +
		"Content-type: text/plain;charset=utf8\n" +
		"Content-transfer-encoding: 8bit\n"

	composeText = strings.Replace(composeText, "@endheaders", endheaders, 1)
	outId := (OutIdentities[identity]).(map[string]interface{})
	from := outId["fromaddr"].(string)
	fromName := outId["fromname"].(string)
	replytoAddr, err := outId["replytoaddr"].(string)
	headerTop := "From: " + fromName + " <" + from + ">\n"
	if !err && replytoAddr != "" {
		headerTop += "Reply-to: <" + replytoAddr + ">\n"
	}

	composeText += addAttach(r, q, "1", boundary) +
		addAttach(r, q, "2", boundary) +
		addAttach(r, q, "3", boundary) +
		addAttach(r, q, "4", boundary)

	composeText = strings.Replace(composeText, identity+"\n", headerTop, 1)
	composeText = strings.ReplaceAll(composeText, "\n", "\r\n")

	var toaddrlist, ccaddrlist string
	fmt.Sscanf(strings.Split(composeText, "To: ")[1], "%s\r\n", &toaddrlist)
	fmt.Sscanf(strings.Split(composeText, "Cc: ")[1], "%s\r\n", &ccaddrlist)
	if ccaddrlist != "" {
		toaddrlist = toaddrlist + "," + ccaddrlist
	}
	toaddr := strings.Split(toaddrlist, ",")
	er := smtp.SendMail(outId["smtphost"].(string),
		smtp.PlainAuth("",
			outId["smtpuser"].(string),
			outId["smtppass"].(string),
			strings.Split(outId["smtphost"].(string), ":")[0]),
		from,
		toaddr,
		[]byte(composeText))
	if er != nil {
		fmt.Fprint(r, "send failed: ", er)
	} else {
		fmt.Fprint(r, "send ok")
	}
	er = ioutil.WriteFile(outId["outfolder"].(string)+separ+boundary, []byte(composeText), 0600)
	if er != nil {
		fmt.Fprint(r, " - copy failed: ", er)
	} else {
		fmt.Fprint(r, " - copy ok")
	}
}

func HdlReply(r http.ResponseWriter, q *http.Request) {
	if !HookAuth(r, q) {
		return
	}

}

func HdlResync(r http.ResponseWriter, q *http.Request) {
	if !HookAuth(r, q) {
		return
	}

	<-ChanSyncerLoop
	ChanSyncerLoop <- 1
	fmt.Fprint(r, "ok")
}

var OutIdentities map[string]interface{}
var ChanSyncerLoop chan int

func main() {
	separ = string(filepath.Separator)
	if os.Getenv("SYNCER") == "1" {
		syncer.SyncerMain()
		return
	}

	viper.SetDefault("ListenAddr", ":1336")
	viper.SetDefault("TLSCert", "cert.pem")
	viper.SetDefault("TLSKey", "key.pem")
	viper.SetDefault("LoginHash", "Y2hhbmdlOnRoaXM=") //change:this
	viper.SetDefault("MaxMessages", 30000)
	viper.SetDefault("StartupCommand", "")
	viper.SetDefault("ReloadCommand", "")

	viper.SetConfigName("CountablyMany")
	viper.AddConfigPath(".")
	viper.ReadInConfig()

	OutIdentities = viper.GetStringMap("OutIdentities")

	SyncerConfig = syncer.ReadConfig()

	SyncerIes = SyncerConfig.ReadIndexEntries()
	ChanSyncerLoop = make(chan int)
	go syncer.SyncerLoop(ChanSyncerLoop)
	http.HandleFunc("/", HdlRes)
	http.HandleFunc("/cmd", HdlCmd)
	http.HandleFunc("/read", HdlRead)
	http.HandleFunc("/replytemplate", HdlReplytemplate)
	http.HandleFunc("/attachget", HdlAttachGet)
	http.HandleFunc("/compose", HdlCompose)
	http.HandleFunc("/send", HdlSend)
	http.HandleFunc("/reply", HdlReply)
	http.HandleFunc("/source", HdlSource)
	http.HandleFunc("/resync", HdlResync)
	if viper.GetString("TLSCert") != "" && viper.GetString("TLSKey") != "" {
		http.ListenAndServeTLS(viper.GetString("ListenAddr"), viper.GetString("TLSCert"), viper.GetString("TLSKey"), nil)
	} else {
		http.ListenAndServe(viper.GetString("ListenAddr"), nil)
	}
}
