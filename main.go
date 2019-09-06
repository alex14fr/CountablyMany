package main

import (
	"bufio"
	"fmt"
	"net/url"
	"github.com/jhillyerd/enmime"
	"html"
	"net/http"
	"os"
	"strings"
	"time"
	"github.com/laochailan/notmuch-go"
	"github.com/spf13/viper"
)

func sendCSP(r http.ResponseWriter) {
	r.Header().Set("Content-Security-Policy", "default-src 'self' ")
}

var Nmdb *notmuch.Database

func OpenNmdb() {
	Nmdb, _ = notmuch.OpenDatabase(viper.GetString("NotmuchDB"), notmuch.DATABASE_MODE_READ_WRITE)
}

func CloseNmdb() {
	Nmdb.Close()
}


func HdlRes(r http.ResponseWriter, q *http.Request) {
	if q.FormValue("q") == "js" {
		http.ServeFile(r,q,"script.js");
	} else if(q.FormValue("q")=="css") {
		http.ServeFile(r,q,"style.css");
	} else {
		sendCSP(r);
		http.ServeFile(r,q,"index.html");
	}
}

func HdlCmd(r http.ResponseWriter, q *http.Request) {
	sendCSP(r)
	limit := viper.GetInt("MaxMessages")
	query:=q.FormValue("q");
	querys:=strings.Split(query,"//")
	AddTags:=[]string{};
	RemoveTags:=[]string{};
	OpenNmdb()
	defer CloseNmdb()
	var tagmod,subject,back string;
	if(len(querys)>1) {
		if(len(querys)>2) {
			tagmod=querys[2]
			subject=querys[1]
			back=querys[0]
		} else {
			tagmod=querys[1]
			subject=querys[0]
			back=querys[0]
		}
		fmt.Println("tagmod="+tagmod+" subject="+subject+" back="+back)
		tagmods:=strings.Split(tagmod," ")
		for _,tag:=range tagmods {
			if(tag[0]=='+') {
				AddTags=append(AddTags,tag[1:]);
			} else {
				RemoveTags=append(RemoveTags,tag[1:]);
			}
		}
		nmqq:=Nmdb.CreateQuery(subject)
		if nmqq==nil {
			fmt.Fprintf(r,"can't create query "+subject)
			return
		}
		msgss:=nmqq.SearchMessages()
		if msgss==nil {
			fmt.Fprintf(r,"Can't search messages query "+subject)
			return
		}
		for ; msgss.Valid() ; msgss.MoveToNext() {
			msgg:=msgss.Get()
			for _,tag:=range AddTags {
				msgg.AddTag(tag);
			}
			for _,tag:=range RemoveTags {
				msgg.RemoveTag(tag);
			}
		}
		if(q.FormValue("onlyretag")!="1") {
			//http.Redirect(r,q,"/?q="+back,302)
		} else {
			fmt.Fprintf(r,"ok")
			return
		}
		query=back
	}
	if query=="" {
		query="tag:inbox";
	}
	nmq := Nmdb.CreateQuery(query)
	if nmq==nil {
		fmt.Fprint(r,"Can't create query "+query)
		return
	}
	dateND := time.Now().Format("02/01/2006")
	i := 0
	msgs := nmq.SearchMessages()
	if(msgs==nil) {
		fmt.Fprintf(r,"Can't search messages, query "+query)
		return
	}
	for ; msgs.Valid() && i < limit; msgs.MoveToNext() {
		i++
		msg := msgs.Get()
		dateU, _ := msg.GetDate()
		datelbl := time.Unix(dateU, 0).Format("02/01/2006")
		dateH := time.Unix(dateU, 0).Format("15:04")
		if datelbl == dateND {
			datelbl = dateH
		}
		from := msg.GetHeader("From")
		fromfmt := strings.Split(strings.ReplaceAll(from, "\"", ""), "<")[0]
		fmt.Fprintf(r, "<div class=msglistRow data-mid='%s'><span>%s</span><span>%s</span><span>%s</span><span>", msg.GetMessageId(), datelbl, fromfmt, html.EscapeString(msg.GetHeader("Subject")))
		for tags:=msg.GetTags() ; tags.Valid() ; tags.MoveToNext() {
			fmt.Fprintf(r, tags.Get()+" ")
		} 
		fmt.Fprintf(r, "</span></div>");
	}
	fmt.Fprintf(r, "</div>")

}

func GetMessageFile(r http.ResponseWriter, q *http.Request) (*os.File,string) {
	id := q.FormValue("id")
	msg, err := Nmdb.FindMessage(id)
	if err != notmuch.STATUS_SUCCESS {
		fmt.Fprint(r, "Message ID "+id+" not found")
		return nil,""
	}
	msg.RemoveTag("unread")
	fname := msg.GetFileName()
	file, err2 := os.Open(fname)
	if err2 != nil {
		fmt.Fprint(r, "Can't open "+fname)
		return nil,""
	}
	return file,fname
}

func HdlSource(r http.ResponseWriter, q *http.Request) {
	r.Header().Set("Content-type","text/plain")
	OpenNmdb()
	defer CloseNmdb()
	_,fname:=GetMessageFile(r,q)
	http.ServeFile(r,q,fname)
}

func HdlRead(r http.ResponseWriter, q *http.Request) {
	sendCSP(r)
	OpenNmdb()
	defer CloseNmdb()
	id := q.FormValue("id")
	file,fname := GetMessageFile(r,q)
	mail, err2 := enmime.ReadEnvelope(bufio.NewReader(file))
	if err2 != nil {
		fmt.Fprint(r, "Can't parse mail id "+id)
		return
	}
	fmt.Fprint(r, "<div id=headers><table><tr><td><b>From</b><td>"+mail.GetHeader("From")+
	"<tr><td><b>To</b><td>"+html.EscapeString(mail.GetHeader("To")+", "+mail.GetHeader("Cc"))+
	"<tr><td><b>Subject</b><td>"+html.EscapeString(mail.GetHeader("Subject"))+
	"<tr><td><b>Date</b><td>"+html.EscapeString(mail.GetHeader("Date"))+"</table>")
	_=fname
	fmt.Fprint(r, "<div id=attachments><a href=/source?id="+url.QueryEscape(id)+" target=_new>src</a>" /* ["+fname+"]" */ +"<br>")
	for _,att:=range append(mail.Attachments,mail.Inlines...) {
		url:="/attachget?id="+url.QueryEscape(id)+"&cid="+url.QueryEscape(att.FileName)
		url1:=url+"&mode=inline"
		url2:=url+"&mode=attach"
		fmt.Fprint(r,"<a href="+url1+" target=_new>"+att.FileName+"</a> ("+att.ContentType+") <a href="+url2+">[dl]</a><br>")
	}


	fmt.Fprint(r,"</div></div><div id=mailbody>")
	htmlmail := string(mail.HTML)
	if htmlmail == "" {
		htmlmail = string(mail.Text)
		htmlmail = strings.ReplaceAll(htmlmail, "\n", "<br>")
	}
	fmt.Fprint(r, htmlmail+"</div>")

}

func HdlCompose(r http.ResponseWriter, q *http.Request) {

}

func HdlReply(r http.ResponseWriter, q *http.Request) {

}

func HdlAttachGet(r http.ResponseWriter, q *http.Request) {
	sendCSP(r)
	OpenNmdb()
	defer CloseNmdb()
	cid := q.FormValue("cid")
	mode := q.FormValue("mode")
	file,_ := GetMessageFile(r,q)
	mail, err2 := enmime.ReadEnvelope(bufio.NewReader(file))
	if err2 != nil {
		fmt.Fprint(r, "Can't parse mail")
		return
	}
	for _,att:=range append(mail.Attachments,mail.Inlines...) {
		if att.FileName==cid {
			r.Header().Set("Content-type: ",att.ContentType)
			if(mode=="attach") {
				r.Header().Set("Content-disposition","attachment;filename=\""+att.FileName+"\"")
			} else {
				r.Header().Set("Content-disposition","inline")
			}
			fmt.Fprintf(r, "%s", att.Content);
			break;
		}
	}
	fmt.Fprint(r, "CID not found in mail")
}

func main() {
	viper.SetDefault("ListenAddr", ":1336")
	viper.SetDefault("NotmuchDB", "/home/al/Mail/")
	viper.SetDefault("MaxMessages", 30000)
	viper.SetConfigName("CountablyMany")
	viper.AddConfigPath(".")
	viper.AddConfigPath(".config")
	viper.ReadInConfig()
	http.HandleFunc("/", HdlRes)
	http.HandleFunc("/cmd", HdlCmd)
	http.HandleFunc("/read", HdlRead)
	http.HandleFunc("/attachget",HdlAttachGet)
	http.HandleFunc("/compose", HdlCompose)
	http.HandleFunc("/reply", HdlReply)
	http.HandleFunc("/source", HdlSource)
	http.ListenAndServe(":1336", nil)
}
