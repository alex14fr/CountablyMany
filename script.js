var hRows={};
var curId=false; 
var gnextId=false; 

function read(id) {
	var x=document.getElementsByClassName('rowselected')
	if(x[0]) x[0].className="msglistRow";
	hRows[id].className=hRows[id].className+" rowselected";
	curId=id;
	document.location.hash=encodeURIComponent(id);
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

function loadmsglist(query) {
	var e=document.getElementById("msglistContainer");
	e.innerHTML="Loading...";
	fetch("/cmd?q="+encodeURIComponent(query)).then(function(response) {
		response.text().then(function(txt) {
			e.innerHTML=txt;
			var rows=document.getElementsByClassName('msglistRow');
			for(var el=0;el<rows.length;el++) {
				var elt=rows[el];
				var nextid=rows[(el+1)%rows.length].getAttribute("data-mid");
				elt.setAttribute("data-nextid",nextid);
				hRows[elt.getAttribute("data-mid")]=elt;
				elt.onclick=function(ee) { 
					read(ee.currentTarget.getAttribute("data-mid")); 
				}
			}
			document.title="("+rows.length+") CountablyMany";
			if(document.location.hash) {
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
	for(i in hRows) {
		if(hRows[i].getAttribute("data-nextid")==curId) {
			read(i);
		}
	}
}

function cmdAndNext(cmd, text) {
	console.log('cmd out next '+curId+' nextid='+gnextId);	
	hRows[curId].innerHTML+="<span>&rarr;"+text+"</span>";
	window.fetch('/cmd?q=mid:'+encodeURIComponent(curId+' '+cmd)+'&onlyretag=1');
	nextMsg();
}

var commandMode=true;
var composeMode=false;

function updCmdModeIndicator() {
	document.getElementById('cmdmodeindicator').style.display=(commandMode?'inline':'none');
}

document.addEventListener("keydown", function(e) {
	if(composeMode) {
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

	else if(e.key=="K") {
		cmdAndNext("//-inbox +killed -todo -wait -done -archive","Killed");
	}

	else if(e.key=="I") {
		cmdAndNext("//+inbox -archive -killed -todo -wait -done","Inboxed");
	}

	else if(e.key=="a") {
		cmdAndNext("//+archive -inbox -todo -wait -done","Archived");
	}

	else if(e.key=="t") {
		cmdAndNext("//-inbox -archive +todo -wait -done","Todo");
	}

	else if(e.key=="w") {
		cmdAndNext("//-inbox -archive +wait -todo -done","Wait");
	}

	else if(e.key=="d") {
		cmdAndNext("//-inbox -archive +done -todo -wait","Done");
	}

	else if(e.key=="/") {
		commandMode=false;
		updCmdModeIndicator();
		var o=document.getElementById('query')
		o.value+='//mid:'+curId+'//';
		o.focus();
	}

	else if(e.key=="q") {
		loadmsglist(document.getElementById("query").value);
	}

	else if(e.key=="Q") {
		fetch("/reload");
	}

	else if(e.key=="Z") {
		document.body.innerHTML="";
		document.location.hash="";
		document.location="https://x:x@"+document.location.host; 
	}

	else if(e.key=="c") {
		window.open('#compose');
	}

	e.preventDefault();
});

document.addEventListener("DOMContentLoaded", function(e) {
	if(document.location.hash=="#compose") {
		toComposeMode();
		return;
	}
	adjustsizes();
	loadmsglist("");
	document.getElementById("cmdForm").addEventListener("submit", function(e) {
		loadmsglist(document.getElementById("query").value);
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
	window.setInterval(function() { if(document.getElementById("query").value=="") { loadmsglist(""); } }, 300000);

});

window.addEventListener("resize", adjustsizes);


function toComposeMode() {
	composeMode=true;
	document.getElementById('msglistContainer').style.display='none';
	document.getElementById('showmsg').style.display='none';
	document.getElementById('cmdForm').style.display='none';
	document.getElementById('composer').style.display='block';

}

