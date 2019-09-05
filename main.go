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
		fmt.Fprint(r, `
		var hRows={};
		var curId=false; 
		var gnextId=false; 
		function read(id) {
			var x=document.getElementsByClassName('rowselected')
			if(x[0]) x[0].className="msglistRow";
			hRows[id].className=hRows[id].className+" rowselected";
			curId=id;
			gnextId=hRows[curId].getAttribute("data-nextid");
			var e=document.getElementById("showmsg"); 
			e.innerHTML='';
			fetch("/read?id="+encodeURIComponent(id)).then(function(response) { 
				response.text().then(function(txt) { 
					if(curId==id) e.innerHTML=txt; 
				}); 
			});
		}
		document.addEventListener("keydown", function(e) {
			if(e.keyCode==46 && e.shiftKey) {
				console.log('delete '+curId+' nextid='+gnextId);	
				var hh=hRows[curId].innerHeight;
				hRows[curId].innerHTML=" -- killed --";
				var dd=document.getElementById("msglistContainer");
				dd.scrollTop=dd.scrollTop+20;
				window.fetch('/?q=mid:'+encodeURIComponent(curId+' //+killed')+'&onlyretag=1');
				read(gnextId);
			}
		});
		document.addEventListener("DOMContentLoaded", function(e) {
			var cont=document.getElementById('msglistContainer');
			cont.style.height=( (window.innerHeight-30)*.4)+'px';
			var cont2=document.getElementById('showmsg');
			cont2.style.height=( (window.innerHeight-30)*.6 )+'px';
			var rows=document.getElementsByClassName('msglistRow');
			for(var el=0;el<rows.length;el++) {
				var elt=rows[el];
				var nextid=rows[(el+1)%rows.length].getAttribute("data-mid");
				elt.setAttribute("data-nextid",nextid);
				hRows[elt.getAttribute("data-mid")]=elt;
				elt.onclick=function(ee) { 
					read(ee.currentTarget.getAttribute("data-mid")); 
				};
			}
			document.title="📧 ("+rows.length+") CountablyMany";
		});
    `)
	} else if q.FormValue("q") == "css" {
		r.Header().Set("Content-type", "text/css")
		fmt.Fprint(r, `body{font-size:10pt;margin:0;padding:0;font-family:monospace}
input{font-size:10pt;width:100%;font-family:monospace}
#topbar { height:25px;   }
		#msglistContainer { height: 250px; overflow-y:scroll;overflow-x:hidden;border-bottom:1px solid black }
#showmsg { height:250px; overflow-y:scroll;overflow-x:hidden }
.msglistRow:nth-child(even) { background-color: #eee; }
.msglistRow span { display:inline-block; white-space:nowrap; text-overflow:ellipsis; overflow:hidden; }
.msglistRow span:nth-child(1) { padding-right: 5px; }
.msglistRow span:nth-child(2) { max-width:450px; color:#007d9c; padding-right: 15px; }
.msglistRow span:nth-child(3) { max-width:550px; padding-right: 15px; }
.msglistRow span:nth-child(4) { color:#9b007c; padding-right: 15px; }
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
	querys:=strings.Split(query," //")
	AddTags:=[]string{};
	RemoveTags:=[]string{};
	OpenNmdb()
	defer CloseNmdb()
	if(len(querys)>1) {
		query=querys[0]
		//fmt.Println("retag query "+querys[1])
		tagmods:=strings.Split(querys[1]," ")
		for _,tag:=range tagmods {
			if(tag[0]=='+') {
				AddTags=append(AddTags,tag[1:]);
			} else {
				RemoveTags=append(RemoveTags,tag[1:]);
			}
		}
		nmqq:=Nmdb.CreateQuery(query)
		if nmqq==nil {
			fmt.Fprintf(r,"can't create query "+query)
			return
		}
		msgss:=nmqq.SearchMessages()
		if msgss==nil {
			fmt.Fprintf(r,"Can't search messages query "+query)
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
			http.Redirect(r,q,"/?q="+query,302)
		} else {
			fmt.Fprintf(r,"ok")
		}
		return

	}
	nmq := Nmdb.CreateQuery(query)
	if nmq==nil {
		fmt.Fprint(r,"Can't create query "+query)
		return
	}
	fmt.Fprintf(r, `<!doctype html>
	<html>
	<head>
	<script src=/res?q=js></script>
	<link rel=stylesheet href=/res?q=css>
	</head>
	<body><div id=topbar><form><input id=query name=q value='`+query+ `'>
	<div id=msglistContainer>`)
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
	mail, err2 := enmime.ReadEnvelope(bufio.NewReader(file))
	if err2 != nil {
		fmt.Fprint(r, "Can't parse mail id "+id)
		return
	}
	fmt.Fprint(r, "<div id=headers><table><tr><td><b>From</b><td>"+mail.GetHeader("From")+
	"<tr><td><b>To</b><td>"+html.EscapeString(mail.GetHeader("To")+", "+mail.GetHeader("Cc"))+
	"<tr><td><b>Subject</b><td>"+html.EscapeString(mail.GetHeader("Subject"))+
	"<tr><td><b>Date</b><td>"+html.EscapeString(mail.GetHeader("Date"))+"</table>")

	fmt.Fprint(r, "<div id=attachments><a href=/source?id="+url.QueryEscape(id)+" target=_new>source</a><br>")
	for _,att:=range append(mail.Attachments,mail.Inlines...) {
		url:="/attachget?id="+url.QueryEscape(id)+"&cid="+url.QueryEscape(att.FileName)
		url1:=url+"&mode=inline"
		url2:=url+"&mode=attach"
		fmt.Fprint(r,"<a href="+url1+" target=_new>"+att.FileName+"</a> ("+att.ContentType+") <a href="+url2+">[dl]</a><br>")
	}


	fmt.Fprint(r,"</div></div>")
	htmlmail := string(mail.HTML)
	if htmlmail == "" {
		htmlmail = string(mail.Text)
		htmlmail = strings.ReplaceAll(htmlmail, "\n", "<br>")
	}
	fmt.Fprint(r, htmlmail)

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
	file := GetMessageFile(r,q)
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
	http.HandleFunc("/", HdlRoot)
	http.HandleFunc("/read", HdlRead)
	http.HandleFunc("/attachget",HdlAttachGet)
	http.HandleFunc("/compose", HdlCompose)
	http.HandleFunc("/reply", HdlReply)
	http.HandleFunc("/res", HdlRes)
	http.HandleFunc("/source", HdlSource)
	http.ListenAndServe(":1336", nil)
}
