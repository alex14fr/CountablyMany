"use strict";

var hRows={};
var curId=false; 
var gnextId=false; 
var lastNotifTime=0;
var idler=true;

function read(id) {
	var x=document.getElementsByClassName('rowselected')
	if(x[0]) x[0].className="msglistRow";
	if(hRows[id])
		hRows[id].className=hRows[id].className+" rowselected";
	curId=id;
	//document.location.hash=encodeURIComponent(id);
	if(!hRows[id]) return;
	gnextId=hRows[curId].getAttribute("data-nextid");
	var e=document.getElementById("showmsg"); 
	e.innerHTML='';
	var viewpmin=hRows[curId].parentElement.scrollTop;
	var viewpmax=viewpmin+hRows[curId].parentElement.clientHeight;
	var elmin=hRows[curId].offsetTop;
	var elmax=elmin+hRows[curId].clientHeight;
	if(elmin<viewpmin) {
		hRows[curId].parentElement.scrollTop=elmin;
	} else if(elmax>viewpmax) {
		hRows[curId].parentElement.scrollTop=elmax-(viewpmax-viewpmin);
	}
	fetch("/read?id="+encodeURIComponent(id)).then(function(response) { 
		response.text().then(function(txt) { 
			if(curId==id) e.innerHTML=txt; 
		}); 
	});
}

var firstElId;

function eltclick(ee) {
	read(ee.currentTarget.getAttribute("data-mid"));
}

function loadmsglist(query) {
	var e=document.getElementById("msglistContainer");
	e.innerHTML="Loading...";
	commandMode=true;
	updCmdModeIndicator();
	hRows={};
	document.getElementById("query").blur();
	var n, sort='';
	if((n=query.indexOf('%'))!=-1) {
		sort=query.substring(n+1);
		query=query.substring(0,n);
	}
	fetch("/cmd?q="+encodeURIComponent(query)+"&sort="+encodeURIComponent(sort)).then(function(response) {
		response.text().then(function(txt) {
			e.innerHTML=txt;
			const rows=document.getElementsByClassName('msglistRow');
			const n=rows.length;
			for(var el=0;el<n;++el) {
				const elt=rows[el];
				const nextid=rows[(el+1)%rows.length].getAttribute("data-mid");
				elt.setAttribute("data-nextid",nextid);
				hRows[elt.getAttribute("data-mid")]=elt;
				if(el==0) firstElId=elt.getAttribute("data-mid");
				elt.onclick=eltclick;
			}
			document.title="("+rows.length+") "+query+" CountablyMany";
			if(document.location.hash && document.location.hash.indexOf(encodeURIComponent(query)>=0)) {
				read(decodeURIComponent(document.location.hash.substr(1,document.location.hash.length)));
			} 
		});
	});
}

function adjustsizes() {
	var wHeight=window.innerHeight-document.getElementById('cmdForm').clientHeight;
	var cont=document.getElementById('msglistContainer');
	cont.style.height=( wHeight*.4)+'px';
	var cont2=document.getElementById('showmsg');
	cont2.style.height=( wHeight*.6 )+'px';
}

function nextMsg() {
	read(gnextId);
}

function prevMsg() {
	for(var i in hRows) {
		if(hRows[i].getAttribute("data-nextid")==curId) {
			read(i);
		}
	}
}

function cmdAndNext(cmd, text) {
	console.log('cmd out next '+curId+' nextid='+gnextId);	
	hRows[curId].innerHTML+="<span>&rarr;"+text+"</span>";
	window.fetch('/cmd?q='+encodeURIComponent(curId+'##'+cmd));
	nextMsg();
}

var commandMode=true;
var composeMode=false;

function updCmdModeIndicator() {
	document.getElementById('cmdmodeindicator').style.display=(commandMode?'inline':'none');
}

document.addEventListener("keydown", function(e) {
	if(e.ctrlKey||composeMode) {
		return;
	}

	if(!commandMode) {
		if(e.key=="Escape") {
			commandMode=true;
			updCmdModeIndicator();
			document.getElementById('query').blur();
		}
		return;
	}

	if(e.key=="i") {
		commandMode=false;
		updCmdModeIndicator();
	}

	else if(e.key==":") {
		commandMode=false;
		updCmdModeIndicator();
		var o=document.getElementById('query');
		o.focus();
		o.select();
	}

	else if(e.key=="ArrowDown") {
		nextMsg();
	}

	else if(e.key=="ArrowUp") {
		prevMsg();
	}

	else if(e.key=="Delete") {
		cmdAndNext("KILL","Killed");
	}

	else if(e.key=="I") {
		cmdAndNext("inbox","Inboxed");
	}

	else if(e.key=="a") {
		cmdAndNext("archive","Archived");
	}

/*
	else if(e.key=="t") {
		cmdAndNext("todo","Todo");
	}

	else if(e.key=="w") {
		cmdAndNext("wait","Wait");
	}

	else if(e.key=="d") {
		cmdAndNext("done","Done");
	}
*/
	else if(e.key=="G") {
		if(firstEltId)
			read(firstEltId);
	}

	else if(e.key=="q") {
		loadmsglist(document.getElementById("query").value);
	}

	else if(e.key=="Q") {
		fetch("/resync").then(x=>x.text().then(function() { 
									document.getElementById("loading").style.display='none';
									loadmsglist(document.getElementById("query").value);
		}));
		document.getElementById("loading").style.display='block';
	}

	else if(e.key=="Z") {
		document.body.innerHTML="";
		document.location.hash="";
		document.location="https://x:x@"+document.location.host; 
	}

	else if(e.key=="c") {
		window.open('#compose');
	}

	else if(e.key=="r") {
		window.open('#compose,r:'+curId);
	}

	else if(e.key=="R") {
		window.open('#compose,all,r:'+curId);
	}


	else if(e.key=="f") {
		window.open('#compose,f:'+curId);
	}

	else if(e.key=="F") {
		window.open('#compose,F:'+curId);
	}

	else if(e.key=="@") {
		navigator.registerProtocolHandler("mailto", "/#compose,%s", "CM");
	}
	e.preventDefault();
});

document.addEventListener("DOMContentLoaded", function(e) {
	if(document.location.hash.indexOf("#compose")>=0) {
		toComposeMode();
		return;
	}
	adjustsizes();
	document.getElementById("cmdForm").addEventListener("submit", function(e) {
		var v=document.getElementById("query").value;
		document.location.hash=encodeURIComponent(v);
		loadmsglist(v);
		e.preventDefault();
	});

	document.getElementById("query").addEventListener("focus",function(e) {
		commandMode=false;
		updCmdModeIndicator();
	});
	document.getElementById("query").addEventListener("blur",function(e) {
		commandMode=true;
		updCmdModeIndicator();
	});

	if(idler) {
		registerEvtsrc();
	}

	if(!document.location.hash) {
		document.location.hash='#'+encodeURIComponent('inbox');
		document.getElementById("query").value="inbox";
		//document.getElementById("cmdForm").submit();
		loadmsglist("inbox");
	} else {
		var query=document.location.hash.substring(1,document.location.hash.length);
		document.getElementById("query").value=query;
		loadmsglist(query);
	}
});

window.addEventListener("resize", adjustsizes);
function registerEvtsrc() {
		var evtSrc=new EventSource("/idler",{withCredentials:true});
		evtSrc.onmessage=() => { 
			if(Date.now()-lastNotifTime>6000) { 
				Notification.requestPermission().then(
				 new Notification("new message in inbox")); 
				lastNotifTime=Date.now(); 
			}
			loadmsglist(document.getElementById("query").value); 
		}
		evtSrc.onerror=() => {
			//alert("idler error");
			location.reload(true);
		}
}

function selDesti(e) {
	console.log(e.target.dataset.to);
	var s=document.getElementById('compose').value;
	var i=s.indexOf('To: ')
	if(s[i+4]=='\n') {
		s=s.substring(0, i+4)+e.target.dataset.to+s.substring(i+4)
	} else {
		var j=s.indexOf('\n', i)
		s=s.substring(0, j)+","+e.target.dataset.to+s.substring(j)
	}
	document.getElementById('compose').value=s;
}

function addDesti(o) {
	var s='';
	for(var i=0; i<o.length; i++) { 
		var l=o[i];
		s+='<a href=#compose id=add'+i+' data-to="'+l+'">'+l+'</a><br>'; 
	}
	document.getElementById('destiB').innerHTML=s; 
	for(var i=0; i<o.length; i++) {
		document.getElementById('add'+i).onclick=selDesti; 
	}
}

function chgDesti() {
	var s=document.getElementById('desti').value;
	if(s.length>3) {
		fetch("/ab?q="+s).then(r=>r.json()).then(addDesti);
	}
}
function toComposeMode() {
	composeMode=true;
	document.getElementById('desti').onkeyup=chgDesti;
	document.getElementById('msglistContainer').style.display='none';
	document.getElementById('showmsg').style.display='none';
	document.getElementById('cmdForm').style.display='none';
	document.getElementById('composer').style.display='block';
	var all=0;
	if(document.location.hash.indexOf(",mailto")>=0) {
		var hashh=decodeURIComponent(document.location.hash);
		var jj=hashh.indexOf(",mailto:");
		var addr=hashh.substr(jj+8);
		fetch("/replytemplate?to="+addr).then(function(response) { response.text().then(function(txt) {
			document.getElementById('compose').innerHTML=txt; }); });
		return;
	}
	if(document.location.hash.indexOf(",all,")>=0)
		all=1;
	var ii=document.location.hash.indexOf(",r:");
	if(ii>=0) {
		var reply2msg=document.location.hash.substr(ii+3,document.location.hash.length);
		fetch("/replytemplate?all="+all+"&id="+reply2msg).then(function(response) {
										response.text().then(function(txt) {
											document.getElementById('compose').innerHTML=txt;
										});      });
	}

	ii=document.location.hash.indexOf(",f:");
	if(ii>=0) {
		var reply2msg=document.location.hash.substr(ii+3,document.location.hash.length);
		fetch("/replytemplate?mode=f&id="+reply2msg).then(function(response) {
										response.text().then(function(txt) {
											document.getElementById('compose').innerHTML=txt;
										});      });
	}
	ii=document.location.hash.indexOf(",F:");
	if(ii>=0) {
		var reply2msg=document.location.hash.substr(ii+3,document.location.hash.length);
		fetch("/replytemplate?mode=f2&id="+reply2msg).then(function(response) {
										response.text().then(function(txt) {
											document.getElementById('compose').innerHTML=txt;
											document.getElementById('attachMessage').value=reply2msg;
										});      });
	}

}


