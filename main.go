package main

import (
	"bufio"
	"fmt"
	"net/url"
	"github.com/jordan-wright/email"
	"html"
	"net/http"
	"os"
	"strings"
	"time"
	//			"github.com/jaytaylor/html2text"
	"github.com/laochailan/notmuch-go"
	"github.com/spf13/viper"
)

func sendCSP(r http.ResponseWriter) {
	r.Header().Set("Content-Security-Policy", "default-src 'self' fonts.gstatic.com fonts.googleapis.com;")
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
		r.Header().Set("Content-type", "text/javascript")
		fmt.Fprint(r, ` function read(id) {
			fetch("/read?id="+encodeURIComponent(id)).then(function(response) { response.text().then(function(txt) { var e=document.getElementById("showmsg"); e.innerHTML=txt; e.scrollTop=0; }); });
			}
		document.addEventListener("DOMContentLoaded", function(e) {
			var cont=document.getElementById('msglistContainer');
			cont.style.height=( (window.innerHeight-30)*.4)+'px';
			var cont2=document.getElementById('showmsg');
			cont2.style.height=( (window.innerHeight-30)*.6 )+'px';
			var rows=document.getElementsByClassName('msglistRow');
			for(el in rows) {
				var elt=rows[el];
				elt.onclick=function(ee) { 
					read(ee.currentTarget.getAttribute("data-mid")); 
					var x=document.getElementsByClassName('rowselected')
					if(x[0]) x[0].className="msglistRow";
					ee.currentTarget.className=ee.currentTarget.className+" rowselected";

					};
			} });
    `)
	} else if q.FormValue("q") == "css" {
		r.Header().Set("Content-type", "text/css")
		fmt.Fprint(r, `body{font-size:11pt;margin:0;padding:0}
input{font-size:11pt;}
#topbar { height:25px;   }
		#msglistContainer { height: 250px; overflow-y:scroll;overflow-x:hidden }
#showmsg { height:250px; overflow-y:scroll;overflow-x:hidden }
.msglistRow:nth-child(even) { background-color: #eee; }
.msglistRow span:nth-child(1) { padding-right: 5px; }
.msglistRow span:nth-child(2) { color:#007d9c; padding-right: 15px; }
//.msglistRow:hover { background-color:#fc0; } 
.rowselected { background-color:#baccdd !important; }
#headers { background-color: #d3dfea; margin-bottom:10px }
`)
	}
}

func HdlRoot(r http.ResponseWriter, q *http.Request) {
	sendCSP(r)
	limit := viper.GetInt("MaxMessages")
	query:=q.FormValue("q");
	if query=="" {
		query="tag:inbox";
	}
	OpenNmdb()
	defer CloseNmdb()
	nmq := Nmdb.CreateQuery(query)
	fmt.Fprintf(r, `<!doctype html>
	<html>
	<head>
	<title>CountablyMany</title>
	</head>
	<script src=/res?q=js></script>
	<link rel=stylesheet href=/res?q=css>
	<link href="https://fonts.googleapis.com/css?family=Raleway&display=swap" rel="stylesheet">
	<body><div id=topbar><form><input id=query name=q value='`+query+ `'><button href=/>&times;</button>
	<div id=msglistContainer>`)
	dateND := time.Now().Format("02/01/2006")
	i := 0
	for msgs := nmq.SearchMessages(); msgs.Valid() && i < limit; msgs.MoveToNext() {
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
		fmt.Fprintf(r, "<div class=msglistRow data-mid='%s'><span>%s</span><span>%s</span><span>%s</span></div>", msg.GetMessageId(), datelbl, fromfmt, html.EscapeString(msg.GetHeader("Subject")))
		/*for tags:=msg.GetTags() ; tags.Valid() ; tags.MoveToNext() {
			fmt.Fprintf(r, tags.Get())
		} */
	}
	fmt.Fprintf(r, "</div><div id=showmsg></div></body></html>")

}

func GetMessageFName(r http.ResponseWriter, q *http.Request) string {
	id := q.FormValue("id")
	msg, err := Nmdb.FindMessage(id)
	if err != notmuch.STATUS_SUCCESS {
		fmt.Fprint(r, "Message ID "+id+" not found")
		return ""
	}
	return msg.GetFileName()
}

func GetMessageFile(r http.ResponseWriter, q *http.Request) *os.File {
	id := q.FormValue("id")
	msg, err := Nmdb.FindMessage(id)
	if err != notmuch.STATUS_SUCCESS {
		fmt.Fprint(r, "Message ID "+id+" not found")
		return nil
	}
	fname := msg.GetFileName()
	file, err2 := os.Open(fname)
	if err2 != nil {
		fmt.Fprint(r, "Can't open "+fname)
		return nil
	}
	return file
}

func HdlSource(r http.ResponseWriter, q *http.Request) {
	r.Header().Set("Content-type","text/plain")
	OpenNmdb()
	defer CloseNmdb()
	fname:=GetMessageFName(r,q)
	http.ServeFile(r,q,fname)
}

func HdlRead(r http.ResponseWriter, q *http.Request) {
	sendCSP(r)
	OpenNmdb()
	defer CloseNmdb()
	id := q.FormValue("id")
	file := GetMessageFile(r,q)
	mail, err2 := email.NewEmailFromReader(bufio.NewReader(file))
	if err2 != nil {
		//fmt.Fprint(r, "Can't parse mail in "+fname)
	}
	fmt.Fprint(r, "<div id=headers><table><tr><td><b>From</b><td>"+mail.From+"<tr><td><b>To</b><td>"+html.EscapeString(strings.Join(mail.To,","))+"<tr><td><b>Cc</b><td>"+html.EscapeString(strings.Join(mail.Cc,","))+"<tr><td><b>Subject</b><td>"+html.EscapeString(mail.Subject)+"<tr><td><b>Date</b><td>"+html.EscapeString(mail.Headers.Get("Date"))+"</table></div>")
	fmt.Fprint(r, "<div id=attachments><a href=/source?id="+url.QueryEscape(id)+" target=_new>source</a><br>")
	i:=0
	fmt.Println(mail.Attachments)
	for _,att:=range mail.Attachments {
		url:="/attachget?mid="+id+"&aid="+string(i)
		url1:=url+"&mode=inline"
		url2:=url+"&mode=attach"
		fmt.Fprint(r,"<a href="+url1+" target=_new>"+att.Filename+"</a> <a href="+url2+">[dl]</a><br>")
		i++
	}
	fmt.Fprint(r,"</div>")
	htmlmail := string(mail.HTML)
	if htmlmail == "" {
		htmlmail = string(mail.Text)
		htmlmail = "<tt>" + strings.ReplaceAll(htmlmail, "\n", "<br>") + "</tt>"
	}
	fmt.Fprint(r, htmlmail)

}

func HdlCompose(r http.ResponseWriter, q *http.Request) {

}

func HdlReply(r http.ResponseWriter, q *http.Request) {

}

func HdlAttachGet(r http.ResponseWriter, q *http.Request) {

}
func main() {
	viper.SetDefault("ListenAddr", ":1336")
	viper.SetDefault("NotmuchDB", "/home/al/Mail/")
	viper.SetDefault("MaxMessages", 30000)
	viper.SetConfigName("CountablyMany")
	viper.AddConfigPath(".")
	viper.AddConfigPath(".config")
	viper.ReadInConfig()
	http.HandleFunc("/", HdlRoot)
	http.HandleFunc("/read", HdlRead)
	http.HandleFunc("/attachget",HdlAttachGet)
	http.HandleFunc("/compose", HdlCompose)
	http.HandleFunc("/reply", HdlReply)
	http.HandleFunc("/res", HdlRes)
	http.HandleFunc("/source", HdlSource)
	http.ListenAndServe(":1336", nil)
}
