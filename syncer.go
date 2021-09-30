package main

import (
	"bufio"
	"github.com/alyu/configparser"
	"github.com/fsnotify/fsnotify"
	"crypto/tls"
	"database/sql"
	"errors"
	"fmt"
	"github.com/jhillyerd/enmime"
	"html"
	"io"
	"io/ioutil"
	"math/rand"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var hash_mutex sync.Mutex
var idler_started map[string]bool
var sync_inbox map[string]bool
var dont_touch_other bool
var db (*sql.DB)

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
		fmt.Print("S: ",s)
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
	conn, err := tls.Dial("tcp", acc["Server"], &tls.Config{})
	if err != nil {
		fmt.Print(err)
		return
	}
	imapconn.Conn = conn
	imapconn.RW = bufio.NewReadWriter(bufio.NewReader(imapconn.Conn), bufio.NewWriter(imapconn.Conn))
	imapconn.ReadLine("")
	imapconn.WriteLine("x login " + acc["User"] + " " + acc["Pass"])
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
		fmt.Println("! Append not ok: ", s, err)
		uid = 0
	}
	return
}

type IndexEntry struct {
	U uint32 // uid
	A string // accountlocalname
	M string // mailboxlocalname;filename is (path in config)/A/M/U
	F string // from
	S string // subject
	D string // date
	I string // message-id
	T string // to
}

func OpenDB() {
	separ = string(filepath.Separator)
	var err error
	db, err = sql.Open("sqlite", GetConf("Path")+separ+"Index.sqlite")
	if err != nil {
		fmt.Println("! DB error unable to open Index.sqlite: ", err)
	}
}

func MakeIEFromFile(filename string) IndexEntry {
	ie := IndexEntry{U: 0, A: "nonexistent-account", M: "nonexistent-mailbox"}
	fil, _ := os.Open(filename)
	env, err := enmime.ReadEnvelope(fil)
	if err != nil {
		fmt.Println("! error reading envelope for ", fil)
		ie.F = "unknown <u@u.tld>"
		ie.S = "unknown subject"
		ie.D = "0"
		ie.I = "unknown.message-id@nonexistent.tld"
		return ie
	}
	//fmt.Println(filename)

	ie.F = env.GetHeader("From")
	ie.S = env.GetHeader("Subject")
	ie.D = env.GetHeader("Date")
	ie.I = env.GetHeader("Message-ID")
	ie.T = env.GetHeader("To")
	return ie
}

func HasMessageIDmbox(mid string, account string, mbox string) bool {
	r := db.QueryRow("select * from messages where i=? and a=? and m=?", mid, account, mbox)
	return (r.Scan() != sql.ErrNoRows)
}

func HasMessageID(mid string, account string) bool {
	r := db.QueryRow("select * from messages where i=? and a=?", mid, account)
	return (r.Scan() != sql.ErrNoRows)
}

type htmlLine struct {
	rTime int64
	rHtml string
}

func ListMessagesHTML(path string, prepath string) string {
	multiboxes := false
	if strings.Index(path, "*") >= 0 {
		multiboxes = true
	}
	a := strings.Split(path, "/")
	if len(a) < 2 {
		return "invalid path"
	}
	account := a[0]
	locmb := a[1]
	dateND := time.Now().Format("02/01/06")
	lines := []htmlLine{}

	var rows (*sql.Rows)

	//db.Exec("pragma journal_mode=wal") //upd

	qry:="select u,a,m,s,d,i,f from messages where m=?"
	if locmb == "sent" {
		qry=strings.Replace(qry,",f",",t",-1)
	}
	if account == "*" {
		rows, _ = db.Query(qry, locmb)
	} else {
		qry=qry+" and a=?"
		rows, _ = db.Query(qry, locmb, account)
	}

	var ie IndexEntry

	for rows.Next() {
		rows.Scan(&ie.U, &ie.A, &ie.M, &ie.S, &ie.D, &ie.I, &ie.F)
		/*upd
		if(ie.F=="" && locmb=="sent") {
			fna:=GetConf("Path")+separ+ie.A+separ+ie.M+separ+strconv.Itoa(int(ie.U))
			fmt.Println("fna=",fna)
			ie2:=MakeIEFromFile(fna)
			fmt.Println("updating",ie.U,ie2.T)
			db.Exec("update messages set t=? where a=? and m=? and u=?", ie2.T, ie.A, ie.M, ie.U)
		} */
		if (account == "*" || ie.A == account) && (locmb == "*" || ie.M == locmb) {
			parsed, err := time.Parse("Mon, _2 Jan 2006 15:04:05 -0700", ie.D)
			if err != nil {
				parsed, err = time.Parse("Mon, _2 Jan 2006 15:04:05 -0700 (MST)", ie.D)
			}
			if err != nil {
				parsed, err = time.Parse("Mon, _2 Jan 06 15:04:05 -0700", ie.D)
			}
			if err != nil {
				parsed, err = time.Parse("Mon, _2 Jan 2006 15:04:05 MST", ie.D)
			}
			if err != nil {
				parsed, err = time.Parse("_2 Jan 2006 15:04:05 -0700", ie.D)
			}
			dateLbl := parsed.Format("02/01/06")
			dateH := parsed.Format("15:04")
			if dateLbl == dateND {
				dateLbl = dateH
			}
			from := ie.F
			from = strings.ReplaceAll(from, "\"", "")
			from = strings.ReplaceAll(from, "  ", " ")
			fromsplit := strings.Split(from, "<")
			if fromsplit[0] != "" || len(fromsplit) < 2 {
				from = fromsplit[0]
			} else {
				from = fromsplit[1]
			}
			curpath := ""
			if multiboxes {
				curpath = "<span>" + ie.A + "/" + ie.M + "</span>"
			}
			pendingMove, _ := ioutil.ReadFile(prepath + separ + ie.A + separ + ie.M + separ + "moves" + separ + strconv.Itoa(int(ie.U)))
			pendingMovestr := string(pendingMove)
			if pendingMovestr != "" {
				pendingMovestr = "<span>&rarr; " + pendingMovestr + "</span>"
			}
			lines = append(lines, htmlLine{rHtml: fmt.Sprintf("<div class=msglistRow data-mid='%s'><span>%s</span><span>%s</span><span>%s</span>%s%s</div>", ie.A+"/"+ie.M+"/"+strconv.Itoa(int(ie.U)), dateLbl, from, html.EscapeString(ie.S), curpath, pendingMovestr),
				rTime: parsed.Unix()})
		}
	}
	s := ""
	sort.Slice(lines, func(i int, j int) bool { return lines[i].rTime > lines[j].rTime })
	for _, l := range lines {
		s = s + l.rHtml
	}
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
	db.Exec("insert into messages (u,a,m,f,s,d,i,t) values (?,?,?,?,?,?,?,?)",
		ie.U, ie.A, ie.M, ie.F, ie.S, ie.D, ie.I, ie.T)
}

func (imc *IMAPConn) AppendFile(accountname string, localmbname string, filename string, allowDup bool, keepOrig bool) error {
	if !allowDup {
		mid := getMidFromFile(filename)
		if mid != "" && HasMessageIDmbox(mid, accountname, localmbname) {
			err := "AppendFile " + filename + " would duplicate Message-ID " + mid + " in index for " + accountname + "/" + localmbname
			fmt.Println(err)
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
			fmt.Println("AppendFile: link error", err)
			return err
		}
		if !keepOrig {
			filenameCopy := strings.ReplaceAll(filename, "appends", "appended")
			fmt.Println("moving ", filename, " to ", filenameCopy)
			err = os.Rename(filename, filenameCopy)
			if err != nil {
				fmt.Println("error renaming: ", err)
			}
		} else {
			fmt.Println("keeping ", filename)
		}
		return nil
	}
	return errors.New("appendFile: no uid returned")
}

func (imc *IMAPConn) AppendFilesInDir(account string, localmbname string, directory string, allowDup bool, keepOrig bool) {
	finfs, _ := ioutil.ReadDir(directory)
	for _, finf := range finfs {
		if !finf.IsDir() {
			fmt.Println("AppendFilesInDir: appending " + finf.Name() + " in " + account + "/" + localmbname + "...")
			imc.AppendFile(account, localmbname, directory+separ+finf.Name(), allowDup, keepOrig)
		}
	}
}

func GetHighestUID(account string, localmbname string) uint32 {
	huid := uint32(0)
	r := db.QueryRow("select MAX(u) from messages where a=? and m=?", account, localmbname)
	r.Scan(&huid)
	return huid
}

func (imc *IMAPConn) FetchNewInMailbox(account string, localmbname string, fromUid uint32) error {
	fmt.Println("Fetch new in mailbox ", account, "/", localmbname, "...")
	if sync_inbox[account] {
		fmt.Println("  already in progress")
	}
	hash_mutex.Lock()
	sync_inbox[account]=true
	hash_mutex.Unlock()
	defer func() { 
		hash_mutex.Lock()
		sync_inbox[account]=false
		hash_mutex.Unlock()
	}()
	if fromUid == 0 {
		fromUid = GetHighestUID(account, localmbname) + 1
	}
	fmt.Println("New is from uid ", fromUid)
	randomtag := "x" + strconv.Itoa(int(rand.Uint64()))
	imc.WriteLine("x examine " + Mailboxes[account][localmbname])
	sss, _ := imc.ReadLine("* OK [UIDVALIDITY")
	var uidvalidity uint32
	fmt.Sscanf(sss, "* OK [UIDVALIDITY %d]", &uidvalidity)
	uidvaliditys := strconv.Itoa(int(uidvalidity))
	storeduidval, _ := ioutil.ReadFile(GetConf("Path") + separ + account + separ + localmbname + separ + "UIDValidity.txt")
	if string(storeduidval) == "" {
		fmt.Println("writing new UIDValidity.txt")
		ioutil.WriteFile(GetConf("Path")+separ+account+separ+localmbname+separ+"UIDValidity.txt", []byte(uidvaliditys), 0600)
	} else if string(storeduidval) != uidvaliditys {
		fmt.Println("Ooops ! storeduidval and uidvalidity mismatch, better do nothing storeduidval=", storeduidval, "uidval=", uidvaliditys)
		return errors.New("storeduidval and uidvalidity mismatch")
	} else {
		fmt.Println("UIDValidity ok")
	}

	uidToFetch:=make([]uint32,65536)
	sizesToFetch:=make([]uint32,65536)
	var d int
	i:=0
	imc.ReadLine("x ")
	imc.WriteLine(randomtag + " uid fetch " + strconv.Itoa(int(fromUid)) + ":* rfc822.size")
	for true {
		ss, _ := imc.ReadLine("")
		if strings.Index(ss, randomtag) == 0 {
			break
		}
		if strings.Index(ss, "FETCH") >=0 {
			fmt.Println("scanning ", ss)
			fmt.Sscanf(ss, "* %d FETCH (UID %d RFC822.SIZE %d)", &d, &uidToFetch[i], &sizesToFetch[i])
			if uidToFetch[i]<fromUid {
				break
			}
			fmt.Println("to fetch %d uid=%d size=%d",i,uidToFetch[i],sizesToFetch[i])
			i++
		}
	}
	nToFetch:=i
	i=0
	for i<nToFetch {
		var uid, leng int
		fmt.Println("fetching %d / %d...\n", i, nToFetch-1)
		imc.WriteLine(randomtag + " uid fetch " + strconv.Itoa(int(uidToFetch[i])) + " rfc822")
		s, _ := imc.ReadLine("")
		if strings.Index(s, randomtag) == 0 {
			break
		}
		fmt.Sscanf(s, "* %d FETCH (UID %d RFC822 {%d", &d, &uid, &leng)
		fmt.Println("got uid:", uid, " length:", leng)
		content := make([]byte, leng)
		_, err := io.ReadAtLeast(imc.RW, content, leng)
		if err != nil {
			fmt.Println("error ReadAtLeast, can't continue : ", err)
			return err
		}
		imc.ReadLine(randomtag)
		fmt.Println("writing to file...")
		err = ioutil.WriteFile(GetConf("Path")+separ+account+separ+localmbname+separ+strconv.Itoa(int(uid)), content, 0600)
		if err != nil {
			fmt.Println("error WriteFile, can't continue : ", err)
			return err
		}
		fmt.Println("inserting into index...")
		ie := MakeIEFromFile(GetConf("Path") + separ + account + separ + localmbname + separ + strconv.Itoa(int(uid)))
		ie.U = uint32(uid)
		ie.A = account
		ie.M = localmbname
		if HasMessageID(ie.I, ie.A) {
			fmt.Println("was already in index (foreign move ?)")
			fmt.Println("keeping both for now")
		}
		dbAppend(ie)
		i++
	}
	return nil
}

func (imc *IMAPConn) BlockIdle(mbox string) (err error) {
	imc.WriteLine("x examine " + mbox)
	imc.ReadLine("x ")
	imc.WriteLine("x idle")
	imc.ReadLine("+ ")
	finished := false
	go func() {
		for !finished {
			fmt.Println("blockidle : sleeping...")
			time.Sleep(12*60*time.Second)
			fmt.Println("blockidle: end sleeping")
			if !finished {
				err:=imc.WriteLine("DONE")
				if err!=nil {
					finished=true
				}
				imc.WriteLine("x idle")
			}
			fmt.Println("blockidle: going back to sleep")
		}
	}()
	for !finished {
		s, err := imc.ReadLine("* ")
		finished = (err != nil) || strings.Contains(s, "EXIST")
	}
	if err != nil {
		return
	}
	imc.WriteLine("DONE")
	_, err = imc.ReadLine("x OK")
	return
}

func (imc *IMAPConn) MoveInMailbox(account string, localmbname string) error {
	path := GetConf("Path") + separ + account + separ + localmbname + separ + "moves"
	fmt.Println("performing moves in ", path, "...")
	mboxselected := false
	finfs, _ := ioutil.ReadDir(path)
	for _, finf := range finfs {
		if !finf.IsDir() {
			if !mboxselected {
				imc.WriteLine("x select " + Mailboxes[account][localmbname])
				imc.ReadLine("x ")
				mboxselected = true
			}
			dest, _ := ioutil.ReadFile(path + separ + finf.Name())
			fmt.Println("moving ", finf.Name(), " to ", string(dest))
			if strings.Index(string(dest), "KILL") == 0 {
				imc.WriteLine("x uid store " + finf.Name() + " flags \\Deleted")
				imc.ReadLine("x ")
				imc.WriteLine("x expunge")
				imc.ReadLine("x ")
				fname := GetConf("Path") + separ + account + separ + localmbname + separ + finf.Name()
				fmt.Println("removing ", fname)
				err := os.Remove(fname)
				if err != nil {
					fmt.Println("removing failed : ", err)
				}
				uid2kill, _ := strconv.Atoi(finf.Name())
				dbDelete(uint32(uid2kill), account, localmbname)
			} else {
				if GetConfS(account+".imap","HasUIDMove")=="1" {
					imc.WriteLine("x uid move " + finf.Name() + " " + Mailboxes[account][string(dest)])
				} else {
					fmt.Println("move by copy and kill...")
					imc.WriteLine("x uid copy " + finf.Name() + " " + Mailboxes[account][string(dest)])
				}
				var d, olduid, uid uint32
				s, _ := imc.ReadLine("x OK")
				fmt.Sscanf(s, "x OK [COPYUID %d %d %d", &d, &olduid, &uid)
				fmt.Println("uid in orig folder is ", olduid, " uid in dest folder is ", uid)
				if GetConfS(account+".imap","HasUIDMove")!="1" && olduid != 0 && uid != 0 {
					olduids := strconv.Itoa(int(olduid))
					imc.WriteLine("x uid store " + olduids + " flags \\Deleted")
					imc.ReadLine("x OK")
					imc.WriteLine("x expunge")
					imc.ReadLine("x OK")
					fmt.Println("killed old")
				}
				newuids := strconv.Itoa(int(uid))
				err := os.Rename(GetConf("Path")+separ+account+separ+localmbname+separ+finf.Name(), GetConf("Path")+separ+account+separ+string(dest)+separ+newuids)
				if err != nil {
					fmt.Println("error during local rename : ", err)
					fmt.Println("local index not updated")
				} else {
					//	dbDelete(olduid, account, localmbname)
					db.Exec("update messages set u=?, m=? where u=? and m=? and a=?",
						uid, string(dest), olduid, localmbname, account)
				}
			}
			os.Remove(path + separ + finf.Name())
		}
	}
	return nil
}

func SyncerMkdirs() {
	separ := string(filepath.Separator)
	OpenDB()
	p := GetConf("Path")
	os.Mkdir(p, 0770)
	for acc,_ := range Mailboxes {
		os.Mkdir(p+separ+acc, 0770)
		for mbox,_ := range Mailboxes[acc] {
			os.Mkdir(p+separ+acc+separ+mbox, 0770)
			os.Mkdir(p+separ+acc+separ+mbox+separ+"moves", 0770)
			os.Mkdir(p+separ+acc+separ+mbox+separ+"appends", 0770)
			os.Mkdir(p+separ+acc+separ+mbox+separ+"appended", 0770)
		}
	}
}

func startIMAPLoop(acc string, wg *sync.WaitGroup) {
	configs, _ := Config.Section(acc+".imap")
	accparam := configs.Options()
	imapconn, err := Login(accparam)
	if err != nil {
		fmt.Println("login error, skipping account ", acc)
	} else {
		for mbox,_ := range Mailboxes[acc] {
			if !sync_inbox[acc] || mbox != "inbox" {
				imapconn.FetchNewInMailbox(acc, mbox, 0)
			}
			imapconn.AppendFilesInDir(acc, mbox, GetConf("Path")+separ+acc+separ+mbox+separ+"appends", false, false)
			imapconn.MoveInMailbox(acc, mbox)
		}
	}
	wg.Done()
}

func IdlerAll() {
	sects, _ := Config.Find(".imap$")
	for _,section := range sects {
		accName:=section.Name()
		accName=strings.Replace(accName,".imap","",-1)
		if idler_started[accName] {
			continue
		}
		go func(acc string, section (*configparser.Section)) {
			hash_mutex.Lock()
			idler_started[acc]=true
			hash_mutex.Unlock()
			imapconn, err := Login(section.Options())
			if err != nil {
				fmt.Println("*** idler first login error, stopping idling for ", acc, " ***")
				hash_mutex.Lock()
				idler_started[acc]=false
				hash_mutex.Unlock()
				return
			}
			for true {
				fmt.Println("IdlerAll: calling BlockIdle")
				err = imapconn.BlockIdle("inbox")
				fmt.Println("IdlerAll: BlockIdle returned")
				if err == nil {
					imapconn.FetchNewInMailbox(acc, "inbox", 0)
				} else {
					fmt.Println("*** idler for ", acc, " relogin...")
					imapconn, err = Login(section.Options())
					if err != nil {
						fmt.Println("*** idler relogin error, stopping idling for ", acc, " ***")
						hash_mutex.Lock()
						idler_started[acc]=false
						hash_mutex.Unlock()
						break
					}
				}
				fmt.Println("IdlerAll: after if-FetchNew/else")
			}
		}(accName,section)
	}
}

func WaitOneIdler() {
	watcher,_:=fsnotify.NewWatcher()
	defer watcher.Close()
	for k,_:=range idler_started {
		fmt.Println("WaitOneIdler : watching",k)
		watcher.Add(GetConf("Path")+separ+k+separ+"inbox")
	}
	for true {
		ev:=<-watcher.Events
		if ev.Op & fsnotify.Create == fsnotify.Create {
			break
		}
	}
	fmt.Println("WaitOneIdler : something happened")
}

func SyncerMain() {
	if dont_touch_other {
		return
	}
	dont_touch_other = true
	separ = string(filepath.Separator)
	SyncerMkdirs()
	fmt.Println("SyncerMain starting at ", time.Now().Format(time.ANSIC))
	var wg sync.WaitGroup
	for acc,_ := range Mailboxes {
		wg.Add(1)
		go startIMAPLoop(acc, &wg)
	}
	wg.Wait()
	fmt.Println("SyncerMain : Starting idlers")
	IdlerAll()
	dont_touch_other = false
	fmt.Println("SyncerMain stopping at ", time.Now().Format(time.ANSIC))
}
