package main

import (
	"bufio"
	"crypto/sha256"
	"crypto/tls"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/jhillyerd/enmime"
	_ "github.com/mattn/go-sqlite3"
	"html"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	// "github.com/pkg/profile"
)

var separ string
var oauthCache map[string]string
var oauthTimestamp map[string]int64

func HookAuth(r http.ResponseWriter, q *http.Request) bool {
	lhash := GetConf("LoginHash")
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

func GetConf(k string) string {
	return GlobConf[k]
}

/*
func HdlFolder(r http.ResponseWriter, q *http.Request) {
	if !HookAuth(r, q) {
		return
	}
	folder := q.FormValue("folder")

	r.Header().Set("Cache-control", "no-store")

	if folder[0] != '/' {
		folder = GetConf("Path")+"/"+folder
	}

}
*/

func Mkdb() {
	os.Remove(GetConf("Path") + separ + "Index.sqlite")
	var err error
	db, err = sql.Open("sqlite3", GetConf("Path")+separ+"Index.sqlite")
	if err != nil {
		fmt.Println("error : " + err.Error())
		return
	}
	db.Exec("pragma journal_mode=wal; CREATE TABLE messages(u integer, a text, m text, f text, s text, d text, i text, t text, ut integer); CREATE INDEX idx1 on messages (m,a); CREATE INDEX idx2 on messages (i,a,m); begin transaction; ")
	for acc, curacc := range Mailboxes {
		for locmb, _ := range curacc {
			path := GetConf("Path") + separ + acc + separ + locmb
			fmt.Println("path=" + path)
			dirents, _ := os.ReadDir(path)
			for _, dirent := range dirents {
				uid, err := strconv.ParseInt(dirent.Name(), 10, 0)
				if err == nil {
					filename := path + separ + dirent.Name()
					fmt.Println("inserting " + filename)
					ie := MakeIEFromFile(filename)
					ie.U = uint32(uid)
					ie.A = acc
					ie.M = locmb
					dbAppend(ie)
				}
			}

		}
	}
	db.Exec("commit; ")
	db.Close()
}

func HdlCmd(r http.ResponseWriter, q *http.Request) {
	if !HookAuth(r, q) {
		return
	}
	query := q.FormValue("q")

	r.Header().Set("Cache-control", "no-store")

	querys := strings.Split(query, "##")
	if len(querys) > 1 {
		subject := querys[0]
		movedest := querys[1]
		subjectspl := strings.Split(subject, "/")
		fnam := GetConf("Path") + separ + subjectspl[0] + separ + subjectspl[1] + separ + "moves" + separ + subjectspl[2]
		cnt := []byte(movedest)
		err := ioutil.WriteFile(fnam, cnt, 0660)
		if err != nil {
			io.WriteString(r, "wrote "+string(cnt)+" in "+fnam+" "+err.Error())
		}
		return
	}

	if strings.Index(query, "/") < 0 {
		query = "*/" + query
	}

	outstr := ListMessagesHTML(query, GetConf("Path"), q.FormValue("sort"))

	io.WriteString(r, outstr)
}

func GetMessageFile(r http.ResponseWriter, q *http.Request) (*os.File, string) {
	id := strings.ReplaceAll(q.FormValue("id"), "..", "")
	fname := GetConf("Path") + separ + id
	file, err2 := os.Open(fname)
	if err2 != nil {
		io.WriteString(r, "Can't open "+fname)
		return nil, ""
	}
	return file, fname
}

func HdlRead(r http.ResponseWriter, q *http.Request) {
	if !HookAuth(r, q) {
		return
	}

	id := q.FormValue("id")
	file, fname := GetMessageFile(r, q)

	if q.FormValue("source") != "" {
		r.Header().Set("Content-type", "text/plain")
		http.ServeFile(r, q, fname)
		return
	}

	mail, err2 := enmime.ReadEnvelope(bufio.NewReader(file))
	if err2 != nil {
		io.WriteString(r, "Can't parse mail id "+id)
		return
	}

	if q.FormValue("html") != "" {
		r.Header().Set("Content-type", "text/html; charset=utf8")
		r.Header().Set("Content-security-policy", "default-src 'none'")

		hdrs := "<table><tr><td><b>From</b><td>" + html.EscapeString(mail.GetHeader("From")) +
			"<tr><td><b>To</b><td>" + html.EscapeString(mail.GetHeader("To")+", "+mail.GetHeader("Cc")) +
			"<tr><td><b>Subject</b><td>" + html.EscapeString(mail.GetHeader("Subject")) +
			"<tr><td><b>Date</b><td>" + html.EscapeString(mail.GetHeader("Date")) + "</table><hr>"

		htmlmail := string(mail.HTML)
		if htmlmail == "" {
			htmlmail = "<pre>" + string(mail.Text) + "</pre>"
			//htmlmail = strings.ReplaceAll(htmlmail, "\n", "<br>")
		}
		htmlmail = strings.ReplaceAll(htmlmail, "<base", "<ignore-base")
		r.Write([]byte(hdrs + htmlmail))
		return
	}

	io.WriteString(r, "<div id=headers><table><tr><td><b>From</b><td>"+html.EscapeString(mail.GetHeader("From"))+
		"<tr><td><b>To</b><td>"+html.EscapeString(mail.GetHeader("To")+", "+mail.GetHeader("Cc"))+
		"<tr><td><b>Subject</b><td>"+html.EscapeString(mail.GetHeader("Subject"))+
		"<tr><td><b>Date</b><td>"+html.EscapeString(mail.GetHeader("Date"))+"</table>")
	_ = fname
	io.WriteString(r, "<div id=attachments><a href=/read?source=1&id="+url.QueryEscape(id)+" target=_new>src</a> <a href=/read?html=1&id="+url.QueryEscape(id)+" target=_new>html</a><br>")
	for _, att := range append(mail.Attachments, mail.Inlines...) {
		url := "/attachget?id=" + url.QueryEscape(id) + "&cid=" + url.QueryEscape(att.FileName)
		url1 := url + "&mode=inline"
		url2 := url + "&mode=attach"
		io.WriteString(r, "<a href="+url1+" target=_new>"+att.FileName+"</a> ("+att.ContentType+") <a href="+url2+" target=_new>[dl]</a><br>")
	}

	io.WriteString(r, "</div></div><div id=mailbody>")
	htmlmail := string(mail.HTML)
	if htmlmail == "" {
		htmlmail = string(mail.Text)
		htmlmail = strings.ReplaceAll(htmlmail, "\n", "<br>")
	}
	htmlmail = strings.ReplaceAll(htmlmail, "<base", "<ignore-base")
	io.WriteString(r, htmlmail+"</div>")

}

func extractAddr(in string) string {
	var addrs []string
	var ss string
	insplt := strings.Split(in+",", ",")
	for _, nm := range insplt {
		if strings.Index(nm, "<") >= 0 {
			ss = strings.Split(nm, "<")[1]
			ss = ss[:len(ss)-1]
		} else {
			ss = nm
		}
		if ss != "" {
			addrs = append(addrs, ss)
		}
	}
	return strings.Join(addrs, ",")
}

func HdlReplytemplate(r http.ResponseWriter, q *http.Request) {
	if !HookAuth(r, q) {
		return
	}

	id := q.FormValue("id")

	if id != "" {
		fwdMode := (q.FormValue("mode") == "f")
		fwdMode2 := (q.FormValue("mode") == "f2")
		replyto := ""
		subjectre := ""
		file, fname := GetMessageFile(r, q)
		_ = fname
		mail, err2 := enmime.ReadEnvelope(bufio.NewReader(file))
		if err2 != nil {
			io.WriteString(r, "Can't parse mail id "+id+" "+err2.Error())
			return
		}
		if !fwdMode && !fwdMode2 {
			replyto = mail.GetHeader("From")
			if mail.GetHeader("Reply-to") != "" {
				replyto = mail.GetHeader("Reply-to")
			}
			if q.FormValue("all") == "1" {
				replyto = replyto + "," + mail.GetHeader("To") + "," + mail.GetHeader("Cc")
			}
			replyto = extractAddr(replyto)
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
		acc := strings.Split(id, "/")[0]
		for sectionNam, sectionVal := range SMTPServ {
			dfltFor := sectionVal["DefaultFor"]
			if strings.Index(dfltFor, acc) >= 0 {
				replyidentity = sectionNam
				break
			}
		}
		io.WriteString(r, replyidentity+"\r\n"+
			"To: "+replyto+"\r\n"+
			"Cc: \r\n"+
			"Subject: "+subjectre+"\r\n")
		if !fwdMode && !fwdMode2 {
			io.WriteString(r, "In-reply-to: "+mail.GetHeader("Message-ID")+"\r\n")
		}
		io.WriteString(r, "References: "+mail.GetHeader("Message-ID")+" "+mail.GetHeader("References")+"\r\n")

		io.WriteString(r, "@endheaders\r\n\r\n\r\n")

		if !fwdMode2 {
			io.WriteString(r, "\r\n--- Original message ---\r\n"+
				"From: "+mail.GetHeader("From")+"\r\n"+
				"To: "+mail.GetHeader("To")+"\r\n"+
				"Cc: "+mail.GetHeader("Cc")+"\r\n"+
				"Subject: "+mail.GetHeader("Subject")+"\r\n"+
				"Date: "+mail.GetHeader("Date")+"\r\n\r\n"+mailtxt)
		}
	} else {
		io.WriteString(r, "@default\r\n"+
			"To: "+q.FormValue("to")+"\r\n"+
			"Cc: \r\n"+
			"Subject: \r\n@endheaders\r\n\r\n")
	}
}

func HdlAttachGet(r http.ResponseWriter, q *http.Request) {
	if !HookAuth(r, q) {
		return
	}

	cid := q.FormValue("cid")
	mode := q.FormValue("mode")
	file, _ := GetMessageFile(r, q)
	mail, err2 := enmime.ReadEnvelope(bufio.NewReader(file))
	if err2 != nil {
		io.WriteString(r, "Can't parse mail")
		return
	}
	for _, att := range append(mail.Attachments, mail.Inlines...) {
		if att.FileName == cid {
			r.Header().Set("Content-Type", att.ContentType+"; name=\""+att.FileName+"\"")
			if mode == "attach" {
				r.Header().Set("Content-Disposition", "attachment; filename=\""+att.FileName+"\"")
			} else {
				r.Header().Set("Content-Disposition", "inline; filename=\""+att.FileName+"\"")
			}
			io.WriteString(r, string(att.Content))
			return
		}
	}
	io.WriteString(r, "CID not found in mail")
}

func headerStr(header string, value string) (s string) {
	if value != "" {
		return header + ": " + value + "\r\n"
	} else {
		return ""
	}
}

func addAttachMessage(q *http.Request, boundary string) string {
	att := q.FormValue("attachMessage")
	if att == "" {
		return ""
	}
	filc, _ := ioutil.ReadFile(GetConf("Path") + separ + att)
	str := "\r\n--" + boundary + "\r\n" +
		"Content-disposition: inline; filename=\"forwarded message.eml\"\r\n" +
		"Content-type: message/rfc822; name=\"forwarded message.eml\"\r\n\r\n" +
		string(filc)

	fmt.Println("addAttachMessage : ", att, filc)
	return str
}

func split(s string, size int) string {
	ss := ""
	for len(s) > 0 {
		if len(s) < size {
			size = len(s)
		}
		ss, s = ss+s[:size]+"\r\n", s[size:]
	}
	return ss
}

func addAttach(r http.ResponseWriter, q *http.Request, suffix string, boundary string) string {
	mpf, mpfh, er := q.FormFile("attach" + suffix)
	if er != nil {
		return ""
	}
	d, _ := ioutil.ReadAll(mpf)
	return "\r\n--" + boundary + "\r\n" +
		"Content-Disposition: attachment; filename=\"" + mpfh.Filename + "\"\r\n" +
		"Content-Type: " + mpfh.Header.Get("Content-Type") + "; name=\"" + mpfh.Filename + "\"\r\n" +
		"Content-Transfer-Encoding: base64\r\n\r\n" +
		split(base64.StdEncoding.EncodeToString(d), 76)
}

func readStr(rw *bufio.ReadWriter) string {
	rw.Flush()
	retstr := ""
	nok := true
	for nok {
		l, err := rw.ReadString('\n')
		if err != nil {
			fmt.Print("readStr error : ")
			fmt.Print(err)
			return "error"
		}
		fmt.Print("readStr : " + l)
		retstr += l
		nok = rw.Reader.Buffered() > 0
	}
	return retstr
}

func checkAttach(q *http.Request, v string) bool {
	_, mpfh, _ := q.FormFile(v)
	//fmt.Print("checkAttach of ", v, " : ", mpfh != nil)
	return mpfh != nil
}

func Sendmail(host string, user string, pass string, from string, to []string, data string) string {
	conn, err := tls.Dial("tcp", host, &tls.Config{})
	if err != nil {
		fmt.Print(err)
		return "dial error"
	}
	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
	readStr(rw)
	rw.WriteString("EHLO x\r\n")
	readStr(rw)
	rw.WriteString("AUTH LOGIN\r\n")
	readStr(rw)
	rw.WriteString(base64.StdEncoding.EncodeToString([]byte(user)) + "\r\n")
	readStr(rw)
	rw.WriteString(base64.StdEncoding.EncodeToString([]byte(pass)) + "\r\n")
	readStr(rw)
	rw.WriteString("MAIL FROM:<" + from + "> BODY=8BITMIME\r\n")
	readStr(rw)
	fmt.Println("RCPT TO = ", to)
	for _, toaddr := range to {
		rw.WriteString("RCPT TO:<" + toaddr + ">\r\n")
		readStr(rw)
	}
	rw.WriteString("DATA\r\n")
	readStr(rw)
	rw.WriteString(data)
	rw.WriteString("\r\n.\r\n")
	retstr := readStr(rw)
	conn.Close()
	return retstr
}

func Sendmail_OAuth(host string, user string, token string, from string, to []string, data string) string {
	conn, err := tls.Dial("tcp", host, &tls.Config{})
	if err != nil {
		fmt.Print(err)
		return "dial error"
	}
	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
	readStr(rw)
	rw.WriteString("ehlo localhost\r\n")
	readStr(rw)

	var w string

	if oauthtok, tokcached := oauthCache[host]; tokcached && oauthTimestamp[host] > time.Now().Unix()-3000 {
		fmt.Println("reusing cached oauth token")
		w = oauthtok
	} else {
		values := url.Values{}
		values.Set("client_id", GetConf("GMailClientId"))
		values.Set("client_secret", GetConf("GMailClientSecret"))
		values.Set("grant_type", "refresh_token")
		values.Set("refresh_token", token)
		resp, err := http.PostForm("https://oauth2.googleapis.com/token", values)

		if err != nil {
			return "error refreshing token" + err.Error()
		}
		var v map[string]interface{}
		decoder := json.NewDecoder(resp.Body)
		if err := decoder.Decode(&v); err != nil {
			return "2error parsing json" + err.Error()
		}
		w = fmt.Sprintf("user=%s\001auth=Bearer %s\001\001", user, v["access_token"].(string))
		w = base64.StdEncoding.EncodeToString([]byte(w))
		oauthCache[host] = w
		oauthTimestamp[host] = time.Now().Unix()
	}

	rw.WriteString("auth xoauth2 " + w + "\r\n")
	readStr(rw)
	rw.WriteString("mail from:<" + from + ">\r\n")
	readStr(rw)
	fmt.Println("RCPT TO = ", to)
	for _, toaddr := range to {
		rw.WriteString("rcpt to:<" + toaddr + ">\r\n")
		readStr(rw)
	}
	rw.WriteString("data\r\n")
	readStr(rw)
	rw.WriteString(data)
	rw.WriteString("\r\n.\r\n")

	retstr := readStr(rw)
	conn.Close()
	return "(oauth) " + retstr
}

func mimeQPEncode(s string) string {
	b := []byte(s)
	nonprintable := false
	r := ""
	for _, x := range b {
		if x > 31 && x < 128 {
			r = r + fmt.Sprintf("%c", x)
		} else {
			nonprintable = true
			r = r + fmt.Sprintf("=%2X", x)
		}
	}
	if nonprintable {
		return "=?UTF-8?Q?" + r + "?="
	} else {
		return r
	}
}

func HdlSend(r http.ResponseWriter, q *http.Request) {
	if !HookAuth(r, q) {
		return
	}

	composeText := q.FormValue("compose")
	composeText = strings.ReplaceAll(composeText, "\r", "")
	headersText := strings.Split(composeText, "@endheaders")[0]

	var identity string
	fmt.Sscanf(composeText, "%s\n", &identity)

	boundary := "b" + strconv.FormatUint(rand.Uint64(), 36) + strconv.FormatUint(rand.Uint64()>>32, 36)
	outId := SMTPServ[identity]
	multipart := checkAttach(q, "attach1") || checkAttach(q, "attach2") || checkAttach(q, "attach3") || checkAttach(q, "attach4") || checkAttach(q, "attach5") || checkAttach(q, "attach6") || q.FormValue("attachMessage") != ""
	ss := sha256.Sum256([]byte(composeText))
	sss := ss[:]
	endheaders := "MIME-Version: 1.0\r\n" +
		"Date: " + time.Now().Format("Mon, 02 Jan 2006 15:04:05 -0700") + "\r\n" +
		"Message-ID: <" + strconv.FormatUint(rand.Uint64(), 36) + "." +
		strconv.FormatUint(binary.LittleEndian.Uint64(sss), 36) +
		"@" + strings.Split(outId["FromAddr"], "@")[1] + ">\r\n" +
		"Content-Transfer-Encoding: 8bit\r\n" +
		"Content-Type: "
	if multipart {
		endheaders += "multipart/mixed; boundary=\"" + boundary + "\"\r\n\r\n" +
			"This is a multipart message in MIME format. \r\n\r\n" +
			"--" + boundary + "\r\n" +
			"Content-Type: text/plain; charset=\"utf8\"\r\n" +
			"Content-Transfer-Encoding: 8bit\r\n"
	} else {
		endheaders += "text/plain; charset=\"utf8\"\r\n"
	}

	//composeText = strings.Replace(composeText, "@endheaders", endheaders, 1)
	before, after, _ := strings.Cut(composeText, "@endheaders")
	composeText = ""
	for _, header := range strings.Split(before, "\n") {
		headername, headerval, found := strings.Cut(header, ": ")
		if !found && header != "" {
			composeText = composeText + header + "\n"
		}
		if headerval != "" && headerval != " " && headerval != "\r" && headerval != "\n" {
			/*b:=new(strings.Builder)
			qpw:=quotedprintable.NewWriter(b)
			qpw.Write([]byte(headerval))
			qpw.Close()
			fmt.Println("after encoding: ",b.String())
			beginenc:=""
			endenc:=""
			if(b.String()!=headerval) {
				beginenc="=?utf8?Q?"
				endenc="?="
			} */
			composeText = composeText + headername + ": " + mimeQPEncode(headerval) + "\r\n" //beginenc+b.String()+endenc+"\r\n"
		}
	}
	composeText = composeText + endheaders + after

	var toaddrlist, ccaddrlist string
	fmt.Sscanf(strings.Split(headersText, "To: ")[1], "%s\n", &toaddrlist)
	spltCC := strings.Split(headersText, "Cc: ")
	if len(spltCC) > 1 {
		fmt.Sscanf(spltCC[1], "%s\r\n", &ccaddrlist)
	} else {
		ccaddrlist = ""
	}
	if ccaddrlist != "" {
		toaddrlist = toaddrlist + "," + ccaddrlist
	}
	toaddr := strings.Split(toaddrlist, ",")

	from := outId["FromAddr"]
	fromName := outId["FromName"]
	replytoAddr, err := outId["ReplyToAddr"]
	headerTop := "From: " + fromName + " <" + from + ">\r\n"
	if !err && replytoAddr != "" {
		headerTop += "Reply-to: <" + replytoAddr + ">\r\n"
	}

	composeText += addAttach(r, q, "1", boundary) +
		addAttach(r, q, "2", boundary) +
		addAttach(r, q, "3", boundary) +
		addAttach(r, q, "4", boundary) +
		addAttach(r, q, "5", boundary) +
		addAttach(r, q, "6", boundary) +
		addAttachMessage(q, boundary)

	composeText = strings.Replace(composeText, identity+"\n", headerTop, 1)
	if multipart {
		composeText += "\r\n--" + boundary + "--\r\n"
	}

	var status string
	if token, tokenpresent := outId["GMailToken"]; tokenpresent {
		fmt.Println("Sendmail using OAuth....")
		status = Sendmail_OAuth(outId["SMTPHost"], outId["SMTPUser"], token, from, toaddr, composeText)
	} else {
		status = Sendmail(outId["SMTPHost"], outId["SMTPUser"], outId["SMTPPass"], from, toaddr, composeText)
	}
	er := ioutil.WriteFile(outId["OutFolder"]+separ+boundary, []byte(composeText), 0600)
	if er != nil {
		io.WriteString(r, status+" - copy failed: "+er.Error())
	} else {
		io.WriteString(r, status+" - copy ok")
	}
}

func HdlResync(r http.ResponseWriter, q *http.Request) {
	if !HookAuth(r, q) {
		return
	}
	r.Header().Set("Cache-control", "no-store")
	fmt.Println("Got resync")
	quickAcc := q.FormValue("quickacc")
	quickMbox := q.FormValue("quickmbox")
	if quickAcc != "" && quickMbox != "" {
		fmt.Println("Quick resync " + quickAcc + "/" + quickMbox)
		SyncerQuick(quickAcc, quickMbox)
	} else {
		SyncerMain()
	}
	io.WriteString(r, "ok")
}

func HdlIdler(r http.ResponseWriter, q *http.Request) {
	if !HookAuth(r, q) {
		return
	}
	r.Header().Set("Content-type", "text/event-stream")
	r.Header().Set("Cache-control", "no-store")
	for true {
		<-syncChan
		r.Write([]byte("data: newmsg\r\n\r\n"))
		r.(http.Flusher).Flush()
	}
}

/*
func HdlTokens(r http.ResponseWriter, q *http.Request) {
	if !HookAuth(r, q) {
		return
	}
	r.Header().Set("Content-type", "text/plain")
	r.Header().Set("Content-disposition", "inline")
	io.WriteString(r, oauthCache)
	io.WriteString(r, oauthTimestamp)
} */

func HdlAbook(r http.ResponseWriter, q *http.Request) {
	if !HookAuth(r, q) {
		return
	}
	r.Header().Set("Content-type", "text/html; charset=utf8")
	rows, _ := db.Query("select distinct f from messages")
	var to string
	addrs := make(sort.StringSlice, 0, 128)
	for rows.Next() {
		rows.Scan(&to)
		to = strings.ToLower(strings.ReplaceAll(to, "\"", ""))
		to = strings.ReplaceAll(to, "  ", " ")
		tosplit := strings.Split(to, "<")
		if len(tosplit) >= 2 {
			to = tosplit[1]
		}
		to = strings.ReplaceAll(to, ">", "")
		if to != "" && strings.Index(to, "noreply") != 0 {
			i := sort.SearchStrings(addrs, to)
			if i == len(addrs) || addrs[i] != to {
				addrs = append(addrs, to)
				addrs.Sort()
			}
		}
	}
	for _, to := range addrs {
		r.Write([]byte("<a href=\"mailto:" + to + "\">" + to + "</a> <a href=\"javascript:navigator.clipboard.writeText('" + to + "')\">[copier]</a><br>"))
	}
	r.Write([]byte("]"))

}

func main() {
	rand.Seed(time.Now().UnixNano())
	//defer profile.Start().Stop()
	separ = string(filepath.Separator)
	var err error
	readConfig()
	if len(os.Args) > 1 && os.Args[1] == "mkdb" {
		Mkdb()
		return
	}
	syncChan = make(chan int)
	oauthCache = make(map[string]string)
	oauthTimestamp = make(map[string]int64)
	SyncerMkdirs()
	go SyncerMain()
	http.HandleFunc("/", HdlRes)
	http.HandleFunc("/cmd", HdlCmd)
	http.HandleFunc("/read", HdlRead)
	http.HandleFunc("/replytemplate", HdlReplytemplate)
	http.HandleFunc("/attachget", HdlAttachGet)
	http.HandleFunc("/send", HdlSend)
	http.HandleFunc("/resync", HdlResync)
	http.HandleFunc("/idler", HdlIdler)
	http.HandleFunc("/abook", HdlAbook)
	err = http.ListenAndServe("127.0.0.1:1336", nil)
	if err != nil {
		fmt.Println(err)
	}
}
