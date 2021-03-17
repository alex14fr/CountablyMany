package syncer

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/jhillyerd/enmime"
	"gopkg.in/yaml.v2"
	"html"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"database/sql"
	_ "modernc.org/sqlite"
)

var separ string
var db (*sql.DB)

type Account struct {
	Server     string
	User       string
	Pass       string
	HasUidmove bool
	Mailboxes  map[string]string
}

type Accounts map[string]Account
type Config struct {
	Path string
	Acc  Accounts
}

type IMAPConn struct {
	Conn *tls.Conn
	RW   *bufio.ReadWriter
}

func (imc *IMAPConn) ReadLine(waitUntil string) (s string, err error) {
	ok := false
	for !ok {
		fmt.Print("S: ")
		s, err = imc.RW.ReadString('\n')
		if err != nil {
			fmt.Println("imap read error : ", err)
			return
		}
		fmt.Print(s)
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

func Login(acc Account) (imapconn *IMAPConn, err error) {
	imapconn = new(IMAPConn)
	conn, err := tls.Dial("tcp", acc.Server, &tls.Config{})
	if err != nil {
		fmt.Print(err)
		return
	}
	imapconn.Conn = conn
	imapconn.RW = bufio.NewReadWriter(bufio.NewReader(imapconn.Conn), bufio.NewWriter(imapconn.Conn))
	imapconn.ReadLine("")
	imapconn.WriteLine("x login " + acc.User + " " + acc.Pass)
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
}

/*
type IndexEntries []IndexEntry

func (c Config) ReadIndexEntries() (ies IndexEntries) {
	ies = IndexEntries{}
	istr, err := ioutil.ReadFile(c.Path + separ + "Index.yaml")
	if err != nil {
		fmt.Println("! index read error: ", err)
		return
	}
	err = yaml.Unmarshal(istr, &ies)
	if err != nil {
		fmt.Println("! index parse error: ", err)
	}
	return
}

func (c Config) WriteIndexEntries(ies IndexEntries) {
	istr, err := yaml.Marshal(&ies)
	if err != nil {
		fmt.Println("! index marshal error: ", err)
		return
	}
	err = ioutil.WriteFile(c.Path+separ+"Index.yaml", istr, 0600)
	if err != nil {
		fmt.Println("! index write error: ", err)
	}
}
*/


func ReadConfig() Config {
	separ = string(filepath.Separator)
	conf := Config{}
	confstr, err := ioutil.ReadFile("Syncer.yaml")
	if err != nil {
		fmt.Println("! config read error: ", err)
	}
	err = yaml.Unmarshal(confstr, &conf)
	if err != nil {
		fmt.Println("! config parse error: ", err)
	}
	db, err = sql.Open("sqlite", conf.Path+separ+"Index.sqlite")
	if err != nil {
		fmt.Println("! config error unable to open Index.sqlite: ", err)
	}
	return conf
}

func MakeIEFromFile(filename string) IndexEntry {
	ie := IndexEntry{U: 0, A: "nonexistent-account", M: "nonexistent-mailbox"}
	fil, _ := os.Open(filename)
	env, err := enmime.ReadEnvelope(fil)
	if err != nil {
		fmt.Println("! error reading envelope for ", fil)
		ie.F="unknown <u@u.tld>";
		ie.S="unknown subject";
		ie.D="0";
		ie.I="unknown.message-id@nonexistent.tld";
		return ie;
	}
	//fmt.Println(filename)

	ie.F = env.GetHeader("From")
	ie.S = env.GetHeader("Subject")
	ie.D = env.GetHeader("Date")
	ie.I = env.GetHeader("Message-ID")
	return ie
}

func HasMessageIDmbox(mid string, account string, mbox string) bool {
	r:=db.QueryRow("select * from messages where i=? and a=? and m=?", mid, account, mbox)
	return(r.Scan() != sql.ErrNoRows)
}

func HasMessageID(mid string, account string) bool {
	r:=db.QueryRow("select * from messages where i=? and a=?", mid, account)
	return(r.Scan() != sql.ErrNoRows)
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

	if account == "*" {
		rows,_ = db.Query("select u,a,m,f,s,d,i from messages where m=?", locmb)
	} else {
		rows,_ = db.Query("select u,a,m,f,s,d,i from messages where m=? and a=?", locmb, account)
	}

	var ie IndexEntry

	for rows.Next() {
		rows.Scan(&ie.U,&ie.A,&ie.M,&ie.F,&ie.S,&ie.D,&ie.I)
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
			if fromsplit[0] != "" || len(fromsplit)<2 {
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
	db.Exec("delete from messages where u=? and a=? and m=?",uid,account,mbox)
}

func dbAppend(ie IndexEntry) {
	db.Exec("insert into messages (u,a,m,f,s,d,i) values (?,?,?,?,?,?,?)",
			ie.U,ie.A,ie.M,ie.F,ie.S,ie.D,ie.I)
}

func (imc *IMAPConn) AppendFile(c Config, accountname string, localmbname string, filename string, allowDup bool, keepOrig bool) error {
	if !allowDup {
		mid := getMidFromFile(filename)
		if mid != "" && HasMessageIDmbox(mid, accountname, localmbname) {
			err := "AppendFile " + filename + " would duplicate Message-ID " + mid + " in index for " + accountname + "/" + localmbname
			fmt.Println(err)
			return errors.New(err)
		}
	}
	fstr, _ := ioutil.ReadFile(filename)
	uid := imc.Append(c.Acc[accountname].Mailboxes[localmbname], string(fstr))
	if uid != 0 {
		ie := MakeIEFromFile(filename)
		ie.U = uid
		ie.A = accountname
		ie.M = localmbname
		dbAppend(ie)
		copyfile := c.Path + separ + accountname + separ + localmbname + separ + strconv.Itoa(int(uid))
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

func (imc *IMAPConn) AppendFilesInDir(c Config, account string, localmbname string, directory string, allowDup bool, keepOrig bool) {
	finfs, _ := ioutil.ReadDir(directory)
	for _, finf := range finfs {
		if !finf.IsDir() {
			fmt.Println("AppendFilesInDir: appending " + finf.Name() + " in " + account + "/" + localmbname + "...")
			imc.AppendFile(c, account, localmbname, directory+separ+finf.Name(), allowDup, keepOrig)
		}
	}
}

func GetHighestUID(account string, localmbname string) uint32 {
	huid := uint32(0)
	r:=db.QueryRow("select MAX(u) from messages where a=? and m=?",account,localmbname)
	r.Scan(&huid)
	return huid
}

func (imc *IMAPConn) FetchNewInMailbox(c Config, account string, localmbname string, fromUid uint32) error {
	fmt.Println("Fetch new in mailbox ", account, "/", localmbname, "...")
	if fromUid == 0 {
		fromUid = GetHighestUID(account, localmbname) + 1
	}
	fmt.Println("New is from uid ", fromUid)
	randomtag := "x" + strconv.Itoa(int(rand.Uint64()))
	imc.WriteLine("x examine " + c.Acc[account].Mailboxes[localmbname])
	sss, _ := imc.ReadLine("* OK [UIDVALIDITY")
	var uidvalidity uint32
	fmt.Sscanf(sss, "* OK [UIDVALIDITY %d]", &uidvalidity)
	uidvaliditys := strconv.Itoa(int(uidvalidity))
	storeduidval, _ := ioutil.ReadFile(c.Path + separ + account + separ + localmbname + separ + "UIDValidity.txt")
	if string(storeduidval) == "" {
		fmt.Println("writing new UIDValidity.txt")
		ioutil.WriteFile(c.Path+separ+account+separ+localmbname+separ+"UIDValidity.txt", []byte(uidvaliditys), 0600)
	} else if string(storeduidval) != uidvaliditys {
		fmt.Println("Ooops ! storeduidval and uidvalidity mismatch, better do nothing storeduidval=", storeduidval, "uidval=", uidvaliditys)
		return errors.New("storeduidval and uidvalidity mismatch")
	} else {
		fmt.Println("UIDValidity ok")
	}

	imc.ReadLine("x ")
	imc.WriteLine(randomtag + " uid fetch " + strconv.Itoa(int(fromUid)) + ":* rfc822.size")
	ss, _ := imc.ReadLine("")
	if strings.Index(ss, randomtag) == 0 {
		fmt.Println("no new message")
		return nil
	}
	var uid uint32
	var leng int
	var d int
	imc.ReadLine(randomtag)
	fmt.Sscanf(ss, "* %d FETCH (UID %d RFC822.SIZE {%d", &d, &uid, &leng)
	fmt.Println("got uid:", uid, " length:", leng)
	if uid < fromUid {
		fmt.Println("uid<fromUid, no new message")
		return nil
	}

	imc.WriteLine(randomtag + " uid fetch " + strconv.Itoa(int(fromUid)) + ":* rfc822")
	end := false
	for !end {
		s, _ := imc.ReadLine("")
		if strings.Index(s, randomtag) == 0 {
			end = true
		} else {
			fmt.Sscanf(s, "* %d FETCH (UID %d RFC822 {%d", &d, &uid, &leng)
			fmt.Println("got uid:", uid, " length:", leng)
			content := make([]byte, leng)
			_, err := io.ReadAtLeast(imc.RW, content, leng)
			if err != nil {
				fmt.Println("error ReadAtLeast, can't continue : ", err)
				return err
			}
			if uid < fromUid {
				fmt.Println("got uid lower than fromUid, skipping")
			} else {
				fmt.Println("writing to file...")
				err = ioutil.WriteFile(c.Path+separ+account+separ+localmbname+separ+strconv.Itoa(int(uid)), content, 0600)
				if err != nil {
					fmt.Println("error WriteFile, can't continue : ", err)
					return err
				}
				fmt.Println("inserting into index...")
				ie := MakeIEFromFile(c.Path + separ + account + separ + localmbname + separ + strconv.Itoa(int(uid)))
				ie.U = uid
				ie.A = account
				ie.M = localmbname
				if HasMessageID(ie.I, ie.A) {
					fmt.Println("was already in index (foreign move ?)")
					fmt.Println("keeping both for now")
				}
				dbAppend(ie)
			}
			imc.ReadLine("")
		}
	}

	return nil
}

func (imc *IMAPConn) MoveInMailbox(c Config, account string, localmbname string) error {
	path := c.Path + separ + account + separ + localmbname + separ + "moves"
	fmt.Println("performing moves in ", path, "...")
	mboxselected := false
	finfs, _ := ioutil.ReadDir(path)
	for _, finf := range finfs {
		if !finf.IsDir() {
			if !mboxselected {
				imc.WriteLine("x select " + c.Acc[account].Mailboxes[localmbname])
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
				fname := c.Path + separ + account + separ + localmbname + separ + finf.Name()
				fmt.Println("removing ", fname)
				err := os.Remove(fname)
				if err != nil {
					fmt.Println("removing failed : ", err)
				}
				uid2kill, _ := strconv.Atoi(finf.Name())
				dbDelete(uint32(uid2kill), account, localmbname)
			} else {
				if c.Acc[account].HasUidmove {
					imc.WriteLine("x uid move " + finf.Name() + " " + c.Acc[account].Mailboxes[string(dest)])
				} else {
					fmt.Println("move by copy and kill...")
					imc.WriteLine("x uid copy " + finf.Name() + " " + c.Acc[account].Mailboxes[string(dest)])
				}
				var d, olduid, uid uint32
				s, _ := imc.ReadLine("x OK")
				fmt.Sscanf(s, "x OK [COPYUID %d %d %d", &d, &olduid, &uid)
				fmt.Println("uid in orig folder is ", olduid, " uid in dest folder is ", uid)
				if !c.Acc[account].HasUidmove && olduid != 0 && uid != 0 {
					olduids := strconv.Itoa(int(olduid))
					imc.WriteLine("x uid store " + olduids + " flags \\Deleted")
					imc.ReadLine("x OK")
					imc.WriteLine("x expunge")
					imc.ReadLine("x OK")
					fmt.Println("killed old")
				}
				newuids := strconv.Itoa(int(uid))
				err := os.Rename(c.Path+separ+account+separ+localmbname+separ+finf.Name(), c.Path+separ+account+separ+string(dest)+separ+newuids)
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
	c := ReadConfig()
	p := c.Path
	os.Mkdir(p, 0770)
	for acc := range c.Acc {
		os.Mkdir(p+separ+acc, 0770)
		for mbox := range c.Acc[acc].Mailboxes {
			os.Mkdir(p+separ+acc+separ+mbox, 0770)
			os.Mkdir(p+separ+acc+separ+mbox+separ+"moves", 0770)
			os.Mkdir(p+separ+acc+separ+mbox+separ+"appends", 0770)
			os.Mkdir(p+separ+acc+separ+mbox+separ+"appended", 0770)
		}
	}
}

func SyncerMain() {
	separ := string(filepath.Separator)
	SyncerMkdirs()
	fmt.Println("SyncerMain starting at ", time.Now().Format(time.ANSIC))
	conf := ReadConfig()
	for acc := range conf.Acc {
		imapconn, err := Login(conf.Acc[acc])
		if err != nil {
			fmt.Println("login error, skipping account ", acc)
		} else {
			for mbox := range conf.Acc[acc].Mailboxes {
				if imapconn.FetchNewInMailbox(conf, acc, mbox, 0) != nil {
					fmt.Println("FetchNewInMailbox returning error, stopping right now")
					return
				}
				imapconn.AppendFilesInDir(conf, acc, mbox, conf.Path+separ+acc+separ+mbox+separ+"appends", false, false)
				imapconn.MoveInMailbox(conf, acc, mbox)
			}
		}
	}
	fmt.Println("SyncerMain stopping at ", time.Now().Format(time.ANSIC))
}

