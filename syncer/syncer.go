package syncer

import ( "fmt"
			"crypto/tls"
			"io/ioutil"
			"path/filepath"
			"io"
			"bufio"
			"strconv"
			"strings"
			"os"
			"errors"
			"html"
			"time"
			"sort"
			"math/rand"
			"gopkg.in/yaml.v2"
			"github.com/jhillyerd/enmime"
			)

var separ string

var SyncerLog string

type Account struct {
	Server string
	User string
	Pass string
	HasUidmove bool
	Mailboxes map[string]string
}

type Accounts map[string]Account
type Config struct {
	Path string
	Acc Accounts
}

type IMAPConn struct {
	Conn *tls.Conn
	RW *bufio.ReadWriter
}

func (imc *IMAPConn) ReadLine(waitUntil string) (s string, err error) {
	ok:=false
	for !ok {
		fmt.Print("S: ")
		s,err=imc.RW.ReadString('\n')
		if err!=nil {
			fmt.Println("imap read error : ",err)
			return 
		}
		fmt.Print(s)
		if(waitUntil=="" || strings.Index(s, waitUntil)==0) {
			ok=true
		}
	}
	return
}
func (imc *IMAPConn) ReadLineDelim(waitUntil string) (sPre,sPost string, err error) {
	s:=""
	sPre=""
	sPost=""
	for true {
		fmt.Print("S: ")
		s,err=imc.RW.ReadString('\n')
		if err!=nil {
			fmt.Println("imap read error : ",err)
			return
		}
		fmt.Print(s)
		if(strings.Index(s, waitUntil)==0) {
			sPost=s
			return
		} else {
			sPre=sPre+s
		}
	}
	return
}


func (imc *IMAPConn) WriteLine(s string) (err error) {
	if strings.Index(s,"x login ")==0 {
		fmt.Print("C: [LOGIN command]\r\n")
	} else {
		fmt.Print("C: "+s+"\r\n")
	}
	_,err=imc.RW.WriteString(s+"\r\n")
	if err!=nil {
		fmt.Println("imap write error : ",err)
		return
	}
	imc.RW.Flush()
	return
}

func Login(acc Account) (imapconn *IMAPConn, err error) {
	imapconn=new(IMAPConn)
	conn,err:=tls.Dial("tcp",acc.Server,&tls.Config{})
	if err!=nil {
		fmt.Print(err)
		return
	}
	imapconn.Conn=conn
	imapconn.RW=bufio.NewReadWriter(bufio.NewReader(imapconn.Conn),bufio.NewWriter(imapconn.Conn))
	imapconn.ReadLine("")
	imapconn.WriteLine("x login "+acc.User+" "+acc.Pass)
	imapconn.ReadLine("x ")
	return
}

func (imc *IMAPConn) Append(remotembname string, content string) (uid uint32) {
	imc.WriteLine("x append "+remotembname+" {"+strconv.Itoa(len(content))+"}")
	imc.RW.WriteString(content+"\r\n")
	imc.RW.Flush()
	s,_:=imc.ReadLine("x ")
	var uu uint32
	_,err:=fmt.Sscanf(s,"x OK [APPENDUID %d %d",&uu,&uid)
	if err!=nil {
		fmt.Println("! Append not ok: ",s,err)
		uid=0
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

type IndexEntries []IndexEntry

func (c Config) ReadIndexEntries() (ies IndexEntries) {
	ies=IndexEntries{}
	istr,err:=ioutil.ReadFile(c.Path+separ+"Index.yaml")
	if err!=nil {
		fmt.Println("! index read error: ",err)
		return
	}
	err=yaml.Unmarshal(istr,&ies)
	if err!=nil {
		fmt.Println("! index parse error: ",err)
	}
	return
}

func (c Config) WriteIndexEntries(ies IndexEntries) {
	istr,err:=yaml.Marshal(&ies)
	if err!=nil {
		fmt.Println("! index marshal error: ",err)
		return
	}
	err=ioutil.WriteFile(c.Path+separ+"Index.yaml",istr,0600)
	if err!=nil {
		fmt.Println("! index write error: ",err)
	}
}


func ReadConfig() (Config) {
	separ=string(filepath.Separator)
	conf:=Config{}
	confstr,err:=ioutil.ReadFile("Syncer.yaml")
	if err!=nil {
		fmt.Println("! config read error: ",err)
	}
	err=yaml.Unmarshal(confstr,&conf)
	if err!=nil {
		fmt.Println("! config parse error: ",err)
	}
	return conf
}

func MakeIEFromFile(filename string) (IndexEntry) {
	ie:=IndexEntry{U:0,A:"nonexistent-account",M:"nonexistent-mailbox"}
	fil,_:=os.Open(filename)
	env,_:=enmime.ReadEnvelope(fil)
	ie.F=env.GetHeader("From")
	ie.S=env.GetHeader("Subject")
	ie.D=env.GetHeader("Date")
	ie.I=env.GetHeader("Message-ID")
	return ie
}

func (ies IndexEntries) HasMessageID(mid string, account string, mbox string) bool {
	for _,ie:=range ies {
		if ie.A==account && ie.M==mbox && ie.I == mid {
			return true
		}
	}
	return false
}
func (ies IndexEntries) searchMessageID(mid string, account string) *IndexEntry {
	for i,ie:=range ies {
		if ie.A==account && ie.I == mid {
			return  &(ies[i])
		}
	}
	return nil
}


type htmlLine struct {
	rTime int64
	rHtml string
}

func (ies IndexEntries) ListMessagesHTML(path string, prepath string) string {
	multiboxes:=false
	if strings.Index(path,"*")>=0 {
		multiboxes=true
	}
	a:=strings.Split(path,"/")
	if len(a)<2  {
		return "invalid path"
	}
	account:=a[0]
	locmb:=a[1]
	dateND:=time.Now().Format("02/01/2006")
	lines:=[]htmlLine{}
	for _,ie:=range ies {
		if (account=="*"||ie.A==account) && (locmb=="*"||ie.M==locmb) {
			parsed,err:=time.Parse("Mon, _2 Jan 2006 15:04:05 -0700",ie.D)
			if err!=nil {
				parsed,_=time.Parse("Mon, _2 Jan 2006 15:04:05 -0700 (MST)",ie.D)
			}
			dateLbl:=parsed.Format("02/01/2006")
			dateH:=parsed.Format("15:04")
			if dateLbl==dateND {
				dateLbl=dateH
			}
			from:=ie.F
			from=strings.Split(strings.ReplaceAll(from,"\"",""),"<")[0]
			curpath:=""
			if multiboxes {
				curpath="<span>"+ie.A+"/"+ie.M+"</span>"
			}
			pendingMove,_:=ioutil.ReadFile(prepath+separ+ie.A+separ+ie.M+separ+"moves"+separ+strconv.Itoa(int(ie.U)))
			pendingMovestr:=string(pendingMove)
			if pendingMovestr!="" {
				pendingMovestr="<span>&rarr; "+pendingMovestr+"</span>";
			}
			lines=append(lines, htmlLine{rHtml:
							fmt.Sprintf("<div class=msglistRow data-mid='%s'><span>%s</span><span>%s</span><span>%s</span>%s%s</div>",ie.A+"/"+ie.M+"/"+strconv.Itoa(int(ie.U)),dateLbl,from,html.EscapeString(ie.S),curpath,pendingMovestr),
												   rTime: parsed.Unix()})
		}
	}
	s:=""
	sort.Slice(lines, func(i int, j int)bool { return lines[i].rTime>lines[j].rTime })
	for _,l:=range lines {
		s=s+l.rHtml
	}
	if s=="" {
		s="No mail."
	}
	return s

}

func getMidFromFile(filename string) string {
	fil,_:=os.Open(filename)
	env,_:=enmime.ReadEnvelope(fil)
	return env.GetHeader("Message-ID")
}

func (imc *IMAPConn) AppendFile(c Config, accountname string, localmbname string, filename string, allowDup bool, keepOrig bool) error {
	if(!allowDup) {
		ies:=c.ReadIndexEntries()
		mid:=getMidFromFile(filename)
		if(mid!="" && ies.HasMessageID(mid,accountname,localmbname)) {
			err:="AppendFile "+filename+" would duplicate Message-ID "+mid+" in index for "+accountname+"/"+localmbname
			fmt.Println(err)
			return errors.New(err)
		}
	}
	fstr,_:=ioutil.ReadFile(filename)
	uid:=imc.Append(c.Acc[accountname].Mailboxes[localmbname], string(fstr))
	if uid!=0 {
		ie:=MakeIEFromFile(filename)
		ie.U=uid
		ie.A=accountname
		ie.M=localmbname
		ies:=c.ReadIndexEntries()
		ies=append(ies,ie)
		c.WriteIndexEntries(ies)
		copyfile:=c.Path+separ+accountname+separ+localmbname+separ+strconv.Itoa(int(uid))
		err:=os.Link(filename, copyfile)
		if err!=nil {
			fmt.Println("AppendFile: link error",err)
			return err
		}
		if !keepOrig {
			filenameCopy:=strings.ReplaceAll(filename,"appends","appended")
			fmt.Println("moving ",filename," to ",filenameCopy)
			err=os.Rename(filename, filenameCopy)
			if err!=nil {
				fmt.Println("error renaming: ",err)
			}
		} else {
			fmt.Println("keeping ",filename)
		}
		return nil
	}
	return errors.New("appendFile: no uid returned")
}

func (imc *IMAPConn) AppendFilesInDir(c Config, account string, localmbname string, directory string, allowDup bool, keepOrig bool) {
	finfs,_:=ioutil.ReadDir(directory)
	for _,finf:=range finfs {
		if !finf.IsDir() {
			fmt.Println("AppendFilesInDir: appending "+finf.Name()+" in "+account+"/"+localmbname+"...")
			imc.AppendFile(c,account,localmbname,directory+separ+finf.Name(),allowDup,keepOrig)
		}
	}
}

func (ies IndexEntries) GetHighestUID(account string, localmbname string) uint32 {
	huid:=uint32(0)
	for _,k := range(ies) {
		if k.A==account && k.M==localmbname && k.U>huid {
			huid=k.U
		}
	}
	return huid
}

func (imc *IMAPConn) FetchNewInMailbox(c Config, account string, localmbname string, fromUid uint32) error {
	fmt.Println("Fetch new in mailbox ",account,"/",localmbname,"...")
	ies:=c.ReadIndexEntries()
	if fromUid==0 {
		fromUid=ies.GetHighestUID(account, localmbname)+1
	}
	fmt.Println("New is from uid ",fromUid)
	randomtag:="x"+strconv.Itoa(int(rand.Uint64()))
	imc.WriteLine("x examine "+c.Acc[account].Mailboxes[localmbname])
	sss,_:=imc.ReadLine("* OK [UIDVALIDITY")
	var uidvalidity uint32
	fmt.Sscanf(sss,"* OK [UIDVALIDITY %d]",&uidvalidity)
	uidvaliditys:=strconv.Itoa(int(uidvalidity))
	storeduidval,_:=ioutil.ReadFile(c.Path+separ+account+separ+localmbname+separ+"UIDValidity.txt")
	if string(storeduidval)=="" {
		fmt.Println("writing new UIDValidity.txt")
		ioutil.WriteFile(c.Path+separ+account+separ+localmbname+separ+"UIDValidity.txt", []byte(uidvaliditys), 0600)
	} else if(string(storeduidval)!=uidvaliditys) {
		fmt.Println("Ooops ! storeduidval and uidvalidity mismatch, better do nothing storeduidval=",storeduidval,"uidval=",uidvaliditys)
		return errors.New("storeduidval and uidvalidity mismatch")
	} else {
		fmt.Println("UIDValidity ok")
	}

	imc.ReadLine("x ")
	imc.WriteLine(randomtag+" uid fetch "+strconv.Itoa(int(fromUid))+":* rfc822.size")
	ss,_:=imc.ReadLine("")
	if strings.Index(ss,randomtag)==0 {
		fmt.Println("no new message")
		return nil
	}
	var uid uint32
	var leng int
	var d int
	imc.ReadLine(randomtag)
	fmt.Sscanf(ss,"* %d FETCH (UID %d RFC822.SIZE {%d",&d,&uid,&leng)
	fmt.Println("got uid:",uid," length:",leng)
	if uid<fromUid {
		fmt.Println("uid<fromUid, no new message")
		return nil
	}

	imc.WriteLine(randomtag+" uid fetch "+strconv.Itoa(int(fromUid))+":* rfc822")
	end:=false
	for !end {
		s,_:=imc.ReadLine("")
		if(strings.Index(s,randomtag)==0) {
			end=true
		} else {
			fmt.Sscanf(s,"* %d FETCH (UID %d RFC822 {%d",&d,&uid,&leng)
			fmt.Println("got uid:",uid," length:",leng)
			content:=make([]byte,leng)
			_,err:=io.ReadAtLeast(imc.RW, content, leng)
			if err!=nil {
				fmt.Println("error ReadAtLeast, can't continue : ",err)
				return err
			}
			if uid<fromUid {
				fmt.Println("got uid lower than fromUid, skipping")
			} else {
				fmt.Println("writing to file...")
				err=ioutil.WriteFile(c.Path+separ+account+separ+localmbname+separ+strconv.Itoa(int(uid)),content,0600)
				if err!=nil {
					fmt.Println("error WriteFile, can't continue : ",err)
					return err
				}
				fmt.Println("inserting into index...")
				ie:=MakeIEFromFile(c.Path+separ+account+separ+localmbname+separ+strconv.Itoa(int(uid)))
				ie.U=uid
				ie.A=account
				ie.M=localmbname
				ies=c.ReadIndexEntries()
				checkAlready:=ies.searchMessageID(ie.I, account)
				if checkAlready!=nil {
					fmt.Println("was already in index, for mbox=",checkAlready.M," (foreign move ?)")
					fmt.Println("keeping both for now")
				} 
				ies=append(ies,ie)
				c.WriteIndexEntries(ies)
			}
			imc.ReadLine("")
		}
	}

	return nil
}

func (imc *IMAPConn) MoveInMailbox(c Config,account string,localmbname string) error {
	path:=c.Path+separ+account+separ+localmbname+separ+"moves"
	fmt.Println("performing moves in ",path,"...")
	mboxselected:=false
	finfs,_:=ioutil.ReadDir(path)
	for _,finf:=range finfs {
		if !finf.IsDir() {
				if !mboxselected {
					imc.WriteLine("x select "+c.Acc[account].Mailboxes[localmbname])
					imc.ReadLine("x ")
					mboxselected=true
				}
				dest,_:=ioutil.ReadFile(path+separ+finf.Name())
				fmt.Println("moving ",finf.Name()," to ",string(dest))
				if strings.Index(string(dest),"KILL")==0 {
					imc.WriteLine("x uid store "+finf.Name()+" flags \\Deleted")
					imc.ReadLine("x ")
					imc.WriteLine("x expunge")
					imc.ReadLine("x ")
					os.Remove(path+separ+account+separ+localmbname+separ+finf.Name())
					ies:=c.ReadIndexEntries()
					for i,ie:=range ies {
						uid2kill,_:=strconv.Atoi(finf.Name())
						if ie.A==account && ie.M==localmbname && ie.U==uint32(uid2kill) {
							ies[i]=ies[len(ies)-1]
							ies=ies[0:len(ies)-1]
							break
						}
					}
					c.WriteIndexEntries(ies)
				} else {
					if c.Acc[account].HasUidmove {
						imc.WriteLine("x uid move "+finf.Name()+" "+c.Acc[account].Mailboxes[string(dest)])
					} else {
						fmt.Println("move by copy and kill...")
						imc.WriteLine("x uid copy "+finf.Name()+" "+c.Acc[account].Mailboxes[string(dest)])
					}
					var d,olduid,uid uint32
					s,_:=imc.ReadLine("x OK")
					fmt.Sscanf(s,"x OK [COPYUID %d %d %d",&d,&olduid,&uid)
					fmt.Println("uid in orig folder is ",olduid, " uid in dest folder is ",uid)
					if !c.Acc[account].HasUidmove && olduid!=0 && uid!=0 {
							olduids:=strconv.Itoa(int(olduid))
							imc.WriteLine("x uid store "+olduids+" flags \\Deleted")
							imc.ReadLine("x OK")
							imc.WriteLine("x expunge")
							imc.ReadLine("x OK")
							fmt.Println("killed old")
					}
					ies:=c.ReadIndexEntries()
					for i,ie:=range ies {
						if ie.A==account && ie.M==localmbname && ie.U==uint32(olduid) {
							ies[i].M=string(dest)
							ies[i].U=uid
							break
						}
					}

					newuids:=strconv.Itoa(int(uid))
					err:=os.Rename(c.Path+separ+account+separ+localmbname+separ+finf.Name(),c.Path+separ+account+separ+string(dest)+separ+newuids)
					if err!=nil {
						fmt.Println("error during local rename : ",err)
						fmt.Println("local index not updated")
					} else {
						c.WriteIndexEntries(ies)
					}
				}
				os.Remove(path+separ+finf.Name())
		}
	}
	return nil
}

func SyncerMkdirs() {
	separ:=string(filepath.Separator)
	c:=ReadConfig()
	p:=c.Path
	os.Mkdir(p,0700)
	for acc:=range c.Acc {
		os.Mkdir(p+separ+acc,0700)
		for mbox:=range c.Acc[acc].Mailboxes {
			os.Mkdir(p+separ+acc+separ+mbox,0700)
			os.Mkdir(p+separ+acc+separ+mbox+separ+"moves",0700)
			os.Mkdir(p+separ+acc+separ+mbox+separ+"appends",0700)
			os.Mkdir(p+separ+acc+separ+mbox+separ+"appended",0700)
		}
	}
}

func SyncerMain() {
	separ:=string(filepath.Separator)
	conf:=ReadConfig()
	for acc:=range conf.Acc {
		imapconn,_:=Login(conf.Acc[acc])
		for mbox:=range conf.Acc[acc].Mailboxes {
			if(imapconn.FetchNewInMailbox(conf,acc,mbox,0)!=nil) {
				fmt.Println("FetchNewInMailbox returning error, stopping right now")
				return
			}
		}
		for mbox:=range conf.Acc[acc].Mailboxes {
			imapconn.AppendFilesInDir(conf,acc,mbox,conf.Path+separ+acc+separ+mbox+separ+"appends",false,false)
		}
		for mbox:= range conf.Acc[acc].Mailboxes {
			imapconn.MoveInMailbox(conf,acc,mbox)
		}
	}
}

func SyncerLoop(c chan int) {
	SyncerMkdirs()
	for true {
		fmt.Println("SyncerLoop starting at ",time.Now().Format(time.ANSIC))
		SyncerMain()
		fmt.Println("SyncerLoop stopping at ",time.Now().Format(time.ANSIC))
		c <- 1
		//time.Sleep(5*time.Minute)
		<- c
	}
}


