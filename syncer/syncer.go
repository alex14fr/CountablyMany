package syncer

import ( "fmt"
			"crypto/tls"
			"io/ioutil"
			"bufio"
			"strconv"
			"strings"
			"os"
			"errors"
			"gopkg.in/yaml.v2"
			"github.com/jhillyerd/enmime"
			)

type Account struct {
	Server string
	User string
	Pass string
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


func (imc *IMAPConn) WriteLine(s string) (err error) {
	fmt.Print("C: "+s+"\r\n")
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
	istr,err:=ioutil.ReadFile(c.Path+"/Index.yaml")
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
	err=ioutil.WriteFile(c.Path+"/Index.yaml",istr,0600)
	if err!=nil {
		fmt.Println("! index write error: ",err)
	}
}


func ReadConfig() (Config) {
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

func getMidFromFile(filename string) string {
	fil,_:=os.Open(filename)
	env,_:=enmime.ReadEnvelope(fil)
	return env.GetHeader("Message-ID")
}

func (imc *IMAPConn) AppendFile(c Config, accountname string, localmbname string, filename string, allowDup bool) error {
	if(!allowDup) {
		ies:=c.ReadIndexEntries()
		mid:=getMidFromFile(filename)
		if(ies.HasMessageID(mid,accountname,localmbname)) {
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
		copyfile:=c.Path+"/"+accountname+"/"+localmbname+"/"+strconv.Itoa(int(uid))
		err:=os.Link(filename, copyfile)
		if err!=nil {
			fmt.Println("AppendFile: link error",err)
			return err
		}
		return nil
	}
	return errors.New("appendFile: no uid returned")
}

func (imc *IMAPConn) AppendFilesInDir(c Config, account string, localmbname string, directory string, allowDup bool) {
	finfs,_:=ioutil.ReadDir(directory)
	for _,finf:=range finfs {
		if !finf.IsDir() {
			fmt.Println("AppendFilesInDir: appending "+finf.Name()+" in "+account+"/"+localmbname+"...")
			imc.AppendFile(c,account,localmbname,directory+"/"+finf.Name(),allowDup)
		}
	}
}

func SyncerMain() {
	conf:=ReadConfig()
	imapconn,_:=Login(conf.Acc["gmail"])

	imapconn.AppendFilesInDir(conf,"gmail","sent","/home/al/Mail/free/Sent/cur",false)

}
