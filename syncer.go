package main

import (
	"bufio"
	"crypto/tls"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jhillyerd/enmime"
	_ "github.com/mattn/go-sqlite3"
	"html"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var db (*sql.DB)
var syncChan (chan int)

type IMAPConn struct {
	Conn *tls.Conn
	RW   *bufio.ReadWriter
}

func (imc *IMAPConn) ReadLine(waitUntil string) (s string, err error) {
	ok := false
	for !ok {
		s, err = imc.RW.ReadString('\n')
		if err != nil {
			fmt.Println("imap read error : ", err)
			return
		}
		fmt.Print("S: ", s)
		if waitUntil == "" || strings.Index(s, waitUntil) == 0 {
			ok = true
		}
	}
	return
}
func (imc *IMAPConn) ReadLineDelim(waitUntil string) (sPre, sPost string, err error) {
	s := ""
	sPre = ""
	sPost = ""
	for true {
		fmt.Print("S: ")
		s, err = imc.RW.ReadString('\n')
		if err != nil {
			fmt.Println("imap read error : ", err)
			return
		}
		fmt.Print(s)
		if strings.Index(s, waitUntil) == 0 {
			sPost = s
			return
		} else {
			sPre = sPre + s
		}
	}
	return
}

func (imc *IMAPConn) WriteLine(s string) (err error) {
	if strings.Index(s, "x login ") == 0 {
		fmt.Print("C: [LOGIN command]\r\n")
	} else if strings.Index(s, "x authenticate ") == 0 {
		fmt.Print("C: [AUTHENTICATE command]\r\n")
		//fmt.Print("C: " + s + "\r\n")
	} else {
		fmt.Print("C: " + s + "\r\n")
	}
	_, err = imc.RW.WriteString(s + "\r\n")
	if err != nil {
		fmt.Println("imap write error : ", err)
		return
	}
	imc.RW.Flush()
	return
}

func Login(acc map[string]string) (imapconn *IMAPConn, err error) {
	imapconn = new(IMAPConn)
	println(acc["Server"])
	if strings.Contains(acc["Server"], "*NO*") {
		println("(skip)")
		return nil, errors.New("skip")
	}
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 5e9}, "tcp", acc["Server"], &tls.Config{})
	if err != nil {
		fmt.Print(err)
		return
	}
	imapconn.Conn = conn
	imapconn.RW = bufio.NewReadWriter(bufio.NewReader(imapconn.Conn), bufio.NewWriter(imapconn.Conn))
	imapconn.ReadLine("")
	var w string
	token, tokenpresent := acc["GMailToken"]
	o365tok, o365present := acc["O365Token"]
	if tokenpresent || o365present {
		if oauthtok, tokcached := oauthCache[acc["Server"]]; tokcached && oauthTimestamp[acc["Server"]] > time.Now().Unix()-3000 {
			fmt.Println("reusing cached oauth token")
			w = oauthtok
		} else {
			fmt.Println("refreshing oauth token")
			values := url.Values{}
			var tokenendpoint string
			if tokenpresent {
				values.Set("client_id", GetConf("GMailClientId"))
				values.Set("client_secret", GetConf("GMailClientSecret"))
				values.Set("grant_type", "refresh_token")
				values.Set("refresh_token", token)
				tokenendpoint="https://oauth2.googleapis.com/token"
			} else if o365present {
				values.Set("client_id", "9e5f94bc-e8a4-4e73-b8be-63364c29d753")
				values.Set("grant_type", "refresh_token")
				values.Set("refresh_token", o365tok)
				tokenendpoint="https://login.microsoftonline.com/common/oauth2/v2.0/token"
			}
			resp, err := http.PostForm(tokenendpoint, values)
			if err != nil {
				println("error refreshing token" + err.Error())
				return nil, err
			}
			var v map[string]interface{}
			decoder := json.NewDecoder(resp.Body)
			if err := decoder.Decode(&v); err != nil {
				println("2error parsing json" + err.Error())
				return nil, err
			}
			actok, actokpresent := v["access_token"]
			if !actokpresent {
				println("access_token not found")
				return nil, errors.New("access_token not found")
			}
			w = "user=" + acc["User"] + "\001auth=Bearer " + actok.(string) + "\001\001"
			w = base64.StdEncoding.EncodeToString([]byte(w))
			oauthCache[acc["Server"]] = w
			oauthTimestamp[acc["Server"]] = time.Now().Unix()
		}
		imapconn.WriteLine("x AUTHENTICATE XOAUTH2 " + w)
		println("token = ", w)
	} else {
		imapconn.WriteLine("x login " + acc["User"] + " " + acc["Pass"])
	}
	imapconn.ReadLine("x ")
	return
}

func (imc *IMAPConn) Append(remotembname string, content string) (uid uint32) {
	imc.WriteLine("x append " + remotembname + " {" + strconv.Itoa(len(content)) + "}")
	imc.RW.WriteString(content + "\r\n")
	imc.RW.Flush()
	s, _ := imc.ReadLine("x ")
	var uu uint32
	_, err := fmt.Sscanf(s, "x OK [APPENDUID %d %d", &uu, &uid)
	if err != nil {
		println("! Append not ok: " + s + err.Error())
		uid = 0
	}
	return
}

type IndexEntry struct {
	U  uint32 // uid
	A  string // accountlocalname
	M  string // mailboxlocalname;filename is (path in config)/A/M/U
	F  string // from
	S  string // subject
	D  string // date
	I  string // message-id
	T  string // to
	UT int64  // unix time
}

func OpenDB() {
	separ = string(filepath.Separator)
	var err error
	db, err = sql.Open("sqlite3", GetConf("Path")+separ+"Index.sqlite")
	if err != nil {
		println("! DB error unable to open Index.sqlite: " + err.Error())
	}
}

func MakeIEFromFile(filename string) IndexEntry {
	ie := IndexEntry{U: 0, A: "nonexistent-account", M: "nonexistent-mailbox"}
	fil, _ := os.Open(filename)
	env, err := enmime.ReadEnvelope(fil)
	if err != nil {
		println("! error reading envelope for " + filename)
		ie.F = "unknown <u@u.tld>"
		ie.S = "unknown subject"
		ie.D = "0"
		ie.I = "unknown.message-id@nonexistent.tld"
		ie.UT = 0
		return ie
	}

	ie.F = env.GetHeader("From")
	ie.S = env.GetHeader("Subject")
	ie.D = env.GetHeader("Date")
	ie.I = env.GetHeader("Message-ID")
	ie.T = env.GetHeader("To")
	ie.UT = ParseDate(ie.D)

	return ie
}

func HasMessageIDmbox(mid string, account string, mbox string) bool {
	r := db.QueryRow("select * from messages where i=? and a=? and m=?", mid, account, mbox)
	return (r.Scan() != sql.ErrNoRows)
}

func ParseDate(date string) int64 {
	parsed, err := time.Parse("Mon, _2 Jan 2006 15:04:05 -0700", date)
	if err != nil {
		parsed, err = time.Parse("Mon, _2 Jan 2006 15:04:05 -0700 (MST)", date)
	}
	if err != nil {
		parsed, err = time.Parse("Mon, _2 Jan 06 15:04:05 -0700", date)
	}
	if err != nil {
		parsed, err = time.Parse("Mon, _2 Jan 2006 15:04:05 MST", date)
	}
	if err != nil {
		parsed, err = time.Parse("_2 Jan 2006 15:04:05 -0700", date)
	}
	return parsed.Unix()
}

func ListMessagesHTML(path string, prepath string, xsort string) string {
	a := strings.Split(path, "/")
	if len(a) < 2 {
		return "invalid path"
	}
	account := a[0]
	locmb := a[1]
	dateND := time.Now().Format("02/01/06")
	lines := make([]string, 0, 32768)

	var rows (*sql.Rows)

	qry := "select distinct u,a,m,s,d,i,f,ut from messages where m=?"
	if locmb == "sent" {
		qry = strings.Replace(qry, ",f", ",t", -1)
	}
	if account == "*" {
		if xsort != "" {
			qry = qry + " order by " + xsort
		} else {
			qry = qry + " order by ut desc"
		}
		rows, _ = db.Query(qry, locmb)
	} else {
		qry = qry + " and a=?"
		if xsort != "" {
			qry = qry + " order by " + xsort
		} else {
			qry = qry + " order by ut desc"
		}
		rows, _ = db.Query(qry, locmb, account)
	}

	var ie IndexEntry

	for rows.Next() {
		rows.Scan(&ie.U, &ie.A, &ie.M, &ie.S, &ie.D, &ie.I, &ie.F, &ie.UT)
		from := ie.F
		from = strings.ReplaceAll(from, "\"", "")
		from = strings.ReplaceAll(from, "  ", " ")
		fromsplit := strings.Split(from, "<")
		if fromsplit[0] != "" || len(fromsplit) < 2 {
			from = fromsplit[0]
		} else {
			from = fromsplit[1]
		}
		curpath := "<span>" + ie.A + "/" + ie.M + "</span>"
		parsed := time.Unix(ie.UT, 0)
		dateLbl := parsed.Format("02/01/06")
		if dateLbl == dateND {
			dateH := parsed.Format("15:04")
			dateLbl = dateH
		}
		pendingMove, _ := ioutil.ReadFile(prepath + separ + ie.A + separ + ie.M + separ + "moves" + separ + strconv.Itoa(int(ie.U)))
		pendingMovestr := string(pendingMove)
		if pendingMovestr != "" {
			pendingMovestr = "<span>&rarr; " + pendingMovestr + "</span>"
		}
		lines = append(lines, "<div class=msglistRow data-mid='"+ie.A+"/"+ie.M+"/"+strconv.Itoa(int(ie.U))+"'><span>"+dateLbl+"</span><span>"+from+"</span><span>"+html.EscapeString(ie.S)+"</span>"+curpath+pendingMovestr+"</div>")
	}
	s := strings.Join(lines, "")
	if s == "" {
		s = "No mail."
	}
	return s
}

func getMidFromFile(filename string) string {
	fil, _ := os.Open(filename)
	env, _ := enmime.ReadEnvelope(fil)
	return env.GetHeader("Message-ID")
}

func dbDelete(uid uint32, account string, mbox string) {
	db.Exec("delete from messages where u=? and a=? and m=?", uid, account, mbox)
}

func dbAppend(ie IndexEntry) {
	db.Exec("insert into messages (u,a,m,f,s,d,i,t,ut) values (?,?,?,?,?,?,?,?,?)",
		ie.U, ie.A, ie.M, ie.F, ie.S, ie.D, ie.I, ie.T, ie.UT)
}

func (imc *IMAPConn) AppendFile(accountname string, localmbname string, filename string, allowDup bool, keepOrig bool) error {
	if !allowDup {
		mid := getMidFromFile(filename)
		if mid != "" && HasMessageIDmbox(mid, accountname, localmbname) {
			err := "AppendFile " + filename + " would duplicate Message-ID " + mid + " in index for " + accountname + "/" + localmbname
			println(err)
			return errors.New(err)
		}
	}
	fstr, _ := ioutil.ReadFile(filename)
	uid := imc.Append(Mailboxes[accountname][localmbname], string(fstr))
	if uid != 0 {
		ie := MakeIEFromFile(filename)
		ie.U = uid
		ie.A = accountname
		ie.M = localmbname
		dbAppend(ie)
		copyfile := GetConf("Path") + separ + accountname + separ + localmbname + separ + strconv.Itoa(int(uid))
		err := os.Link(filename, copyfile)
		if err != nil {
			println("AppendFile: link error" + err.Error())
			return err
		}
		if !keepOrig {
			filenameCopy := strings.ReplaceAll(filename, "appends", "appended")
			println("moving " + filename + " to " + filenameCopy)
			err = os.Rename(filename, filenameCopy)
			if err != nil {
				println("error renaming: " + err.Error())
			}
		} else {
			println("keeping " + filename)
		}
		return nil
	}
	return errors.New("appendFile: no uid returned")
}

func (imc *IMAPConn) AppendFilesInDir(account string, localmbname string, directory string, allowDup bool, keepOrig bool) {
	finfs, _ := ioutil.ReadDir(directory)
	for _, finf := range finfs {
		if !finf.IsDir() {
			println("AppendFilesInDir: appending " + finf.Name() + " in " + account + "/" + localmbname + "...")
			imc.AppendFile(account, localmbname, directory+separ+finf.Name(), allowDup, keepOrig)
		}
	}
}

func GetHighestUID(account string, localmbname string) uint32 {
	huid := uint32(0)
	r := db.QueryRow("select MAX(u) from messages where a=? and m=?", account, localmbname)
	r.Scan(&huid)
	println("GetHighestUID a=" + account + " m=" + localmbname + " = " + strconv.Itoa(int(huid)))
	return huid
}

func (imc *IMAPConn) FetchNewInMailbox(account string, localmbname string, fromUid uint32) error {
	println("Fetch new in mailbox " + account + "/" + localmbname + "...")
	if fromUid == 0 {
		fromUid = GetHighestUID(account, localmbname) + 1
	}
	println("New is from uid ", fromUid)
	randomtag := "x" + strconv.Itoa(int(rand.Uint64()))
	imc.WriteLine("x examine \"" + Mailboxes[account][localmbname]+"\"")
	sss, _ := imc.ReadLine("* OK [UIDVALIDITY")
	var uidvalidity uint32
	fmt.Sscanf(sss, "* OK [UIDVALIDITY %d]", &uidvalidity)
	uidvaliditys := strconv.Itoa(int(uidvalidity))
	storeduidval, _ := ioutil.ReadFile(GetConf("Path") + separ + account + separ + localmbname + separ + "UIDValidity.txt")
	if string(storeduidval) == "" {
		println("writing new UIDValidity.txt")
		ioutil.WriteFile(GetConf("Path")+separ+account+separ+localmbname+separ+"UIDValidity.txt", []byte(uidvaliditys), 0600)
	} else if string(storeduidval) != uidvaliditys {
		println("Ooops ! storeduidval and uidvalidity mismatch, better do nothing storeduidval=", storeduidval, "uidval=", uidvaliditys)
		return errors.New("storeduidval and uidvalidity mismatch")
	} else {
		println("UIDValidity ok")
	}

	uidToFetch := make([]uint32, 0, 65536)
	var d int
	imc.ReadLine("x ")
	imc.WriteLine(randomtag + " uid fetch " + strconv.Itoa(int(fromUid)) + ":* rfc822.size")
	for true {
		ss, _ := imc.ReadLine("")
		if strings.Index(ss, randomtag) == 0 {
			break
		}
		if strings.Index(ss, "FETCH") >= 0 {
			println("scanning ", ss)
			var xx, yy int
			var x, y uint32
			i1 := strings.Index(ss, "(")
			i2 := strings.Index(ss, ")")
			ss=ss[i1:i2]
			sss := strings.Split(ss, " ")
			if strings.Contains(sss[0], "UID") {
				xx, _=strconv.Atoi(sss[1])
				yy, _=strconv.Atoi(sss[3])
			} else {
				xx, _=strconv.Atoi(sss[3])
				yy, _=strconv.Atoi(sss[1])
			}
			x=uint32(xx)
			y=uint32(yy)
			//fmt.Sscanf(ss, "* %d FETCH (UID %d RFC822.SIZE %d)", &d, &x, &y)
			if x < fromUid {
				println("breaking !")
				break
			}
			uidToFetch = append(uidToFetch, x)
			println("to fetch uid=", x, "size=", y)
		}
	}
	cnt, err := os.ReadFile(GetConf("Path") + separ + account + separ + localmbname + separ + "tofetch")
	if err == nil {
		cnts := strings.Split(string(cnt), "\n")
		for _, cntt := range cnts {
			if len(cntt)>0 {
				x, err := strconv.Atoi(cntt)
				if err == nil {
					uidToFetch=append(uidToFetch, uint32(x))
					println("add to fetch uid=", x)
				}
			}
		}
		os.Remove(GetConf("Path") + separ + account + separ + localmbname + separ + "tofetch")
	}
	for i, curUid := range uidToFetch {
		var uid, leng int
		println("fetching ", i+1, "/ ", len(uidToFetch), "...")
		imc.WriteLine(randomtag + " uid fetch " + strconv.Itoa(int(curUid)) + " rfc822")
		s, _ := imc.ReadLine("* ")
		fmt.Sscanf(s, "* %d FETCH (UID %d RFC822 {%d", &d, &uid, &leng)
		if uid==0 && leng==0 {
			fmt.Sscanf(s, "* %d FETCH (RFC822 {%d", &d, &leng)
			uid=int(curUid)
		}
		println("got uid:", uid, " length:", leng)
		content := make([]byte, leng)
		_, err := io.ReadAtLeast(imc.RW, content, leng)
		if err != nil {
			println("error ReadAtLeast, can't continue : ", err)
			return err
		}
		imc.ReadLine(randomtag)
		println("writing to file...")
		err = ioutil.WriteFile(GetConf("Path")+separ+account+separ+localmbname+separ+strconv.Itoa(int(uid)), content, 0600)
		if err != nil {
			println("error WriteFile, can't continue : ", err)
			return err
		}
		println("inserting into index...")
		ie := MakeIEFromFile(GetConf("Path") + separ + account + separ + localmbname + separ + strconv.Itoa(int(uid)))
		ie.U = uint32(uid)
		ie.A = account
		ie.M = localmbname
		dbAppend(ie)
	}
	return nil
}

func (imc *IMAPConn) MoveInMailbox(account string, localmbname string) error {
	path := GetConf("Path") + separ + account + separ + localmbname + separ + "moves"
	println("performing moves in ", path, "...")
	mboxselected := false
	finfs, _ := ioutil.ReadDir(path)
	doExpunge := false
	for _, finf := range finfs {
		if !finf.IsDir() {
			if !mboxselected {
				imc.WriteLine("x select \"" + Mailboxes[account][localmbname]+"\"")
				imc.ReadLine("x ")
				mboxselected = true
			}
			dest, _ := ioutil.ReadFile(path + separ + finf.Name())
			println("moving ", finf.Name(), " to ", string(dest))
			if strings.Index(string(dest), "KILL") == 0 {
				imc.WriteLine("x uid store " + finf.Name() + " flags \\Deleted")
				imc.ReadLine("x ")
				doExpunge = true
				fname := GetConf("Path") + separ + account + separ + localmbname + separ + finf.Name()
				println("removing ", fname)
				err := os.Remove(fname)
				if err != nil {
					println("removing failed : ", err)
				}
				uid2kill, _ := strconv.Atoi(finf.Name())
				dbDelete(uint32(uid2kill), account, localmbname)
			} else {
				if Mailboxes[account]["HasUIDMove"] == "1" {
					imc.WriteLine("x uid move " + finf.Name() + " \"" + Mailboxes[account][string(dest)] + "\"")
				} else {
					println("move by copy and kill...")
					imc.WriteLine("x uid copy " + finf.Name() + " \"" + Mailboxes[account][string(dest)] + "\"")
				}
				var d, olduid, uid uint32
				s, _ := imc.ReadLine("x OK")
				fmt.Sscanf(s, "x OK [COPYUID %d %d %d", &d, &olduid, &uid)
				println("uid in orig folder is ", olduid, " uid in dest folder is ", uid)
				if Mailboxes[account]["HasUIDMove"] != "1" && olduid != 0 && uid != 0 {
					olduids := strconv.Itoa(int(olduid))
					imc.WriteLine("x uid store " + olduids + " flags \\Deleted")
					imc.ReadLine("x OK")
					doExpunge = true
					println("killed old")
				}
				newuids := strconv.Itoa(int(uid))
				err := os.Rename(GetConf("Path")+separ+account+separ+localmbname+separ+finf.Name(), GetConf("Path")+separ+account+separ+string(dest)+separ+newuids)
				if err != nil {
					println("error during local rename : ", err)
					println("local index not updated")
				} else {
					db.Exec("update messages set u=?, m=? where u=? and m=? and a=?",
						uid, string(dest), olduid, localmbname, account)
				}
			}
			os.Remove(path + separ + finf.Name())
		}
	}
	if doExpunge {
		imc.WriteLine("x expunge")
		imc.ReadLine("x ")
	}
	return nil
}

func SyncerMkdirs() {
	separ := string(filepath.Separator)
	OpenDB()
	p := GetConf("Path")
	os.Mkdir(p, 0770)
	for acc, _ := range Mailboxes {
		os.Mkdir(p+separ+acc, 0770)
		for mbox, _ := range Mailboxes[acc] {
			os.Mkdir(p+separ+acc+separ+mbox, 0770)
			os.Mkdir(p+separ+acc+separ+mbox+separ+"moves", 0770)
			os.Mkdir(p+separ+acc+separ+mbox+separ+"appends", 0770)
			os.Mkdir(p+separ+acc+separ+mbox+separ+"appended", 0770)
		}
	}
}

func startIMAPLoop(acc string) {
	accparam := IMAPServ[acc]
	imapconn, err := Login(accparam)
	if err != nil {
		println("login error, skipping account ", acc)
	} else {
		for mbox, _ := range Mailboxes[acc] {
			imapconn.FetchNewInMailbox(acc, mbox, 0)
			imapconn.AppendFilesInDir(acc, mbox, GetConf("Path")+separ+acc+separ+mbox+separ+"appends", false, false)
			imapconn.MoveInMailbox(acc, mbox)
		}
	}
	if imapconn !=nil && imapconn.Conn != nil {
		imapconn.Conn.Close()
	}
}

func SyncerMain() {
	separ = string(filepath.Separator)
	SyncerMkdirs()
	println("SyncerMain starting at ", time.Now().Format(time.ANSIC))
	for acc, _ := range Mailboxes {
		startIMAPLoop(acc)
	}
	println("SyncerMain stopping at ", time.Now().Format(time.ANSIC))
	syncChan <- 1
}

func SyncerQuick(acc string, mbox string) {
	accparam := IMAPServ[acc]
	imapconn, err := Login(accparam)
	if err != nil {
		println("login error, skipping account ", acc)
	} else {
		imapconn.FetchNewInMailbox(acc, mbox, 0)
	}
	if imapconn.Conn != nil {
		imapconn.Conn.Close()
	}
	syncChan <- 1
}
