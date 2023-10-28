<?php
function readl($fh) {
	global $connectionok;
	$ret=[];
	do {
		$l=fgets($fh);
		$ret[]=$l;
//		print "< $l";
	} while($l!==false && !feof($fh) && substr($l,0,2)!="x " && substr($l,0,2)!="+ ");
	return($ret);
}

function list_srv($fh, $config, $mbox) {
	fwrite($fh, "x examine $mbox\r\n");
	readl($fh);
	fwrite($fh, "x uid search all\r\n");
	$r=readl($fh);
	foreach($r as $rr) {
		$rr=trim($rr);
		if(strpos($rr, "* SEARCH")===0) {
			return(explode(' ', substr($rr, 9)));
		}
	}
}

function list_db($db, $config, $mbox) {
	$ret=[];
	$res=$db->query("SELECT u FROM messages WHERE a='".$db->escapeString($config['cmAcc'])."' and m='".$db->escapeString($mbox)."' ORDER BY u");
	while($a=$res->fetchArray(SQLITE3_NUM)) $ret[]=$a[0];
	return($ret);
}

function prune_db($db, $config, $mbox, $list) {
	$qry="";
	foreach($list as $id) {
		$qry.="DELETE FROM messages WHERE a='".$db->escapeString($config['cmAcc'])."' and m='".$db->escapeString($mbox)."' and u=".$id.";";
	}
	$db->exec($qry);
	return($qry);
}

function mkdir_if_notexist($dir) {
	return "[ -d \"$dir\" ] || mkdir \"$dir\"; ";
}

function prune_disk($config, $mbox, $list) {
	global $pruned, $maildir;
	$cmd=mkdir_if_notexist($pruned).mkdir_if_notexist($pruned."/".$config['cmAcc']).mkdir_if_notexist($pruned."/".$config['cmAcc']."/".$mbox);
	foreach($list as $id) {
		$cmd.="mv $maildir/".$config['cmAcc']."/$mbox/$id $pruned/".$config['cmAcc']."/$mbox/; ";
	}
	system($cmd);
	return($cmd);
}

function isnum($s) {
	$n=strlen($s);
	for($i=0; $i<$n; $i++)
		if($s[$i] > '9' || $s[$i] < '0') 
			return(false);
	return(true);
}

function list_disk($config, $mbox) {
	global $maildir;
	$mailname=$maildir."/".$config['cmAcc']."/".$mbox;
	$d=opendir($mailname);
	$ret=[];
	while(($e=readdir($d))!==false) {
		if(isnum($e)) {
			$ret[]=$e;
		}
	}
	//sort($ret);
	return($ret);
}

function main_srv($config) {
	global $maildir;
	$db=new SQLite3($maildir."/Index.sqlite");
	global $cmd, $connectionok;
	print "Connecting to ".$config['host']."... \n";
	$fh=stream_socket_client("tls://".$config['host']);
	if(!$fh) {
		print "Error connecting\n";
		exit(1);
	}
	if(isset($config['xoauth2_cmd']) && !isset($config['xoauth2_enc_token'])) {
		print "Fetching xoauth2 token...";
		$config['xoauth2_enc_token']=base64_encode("user=".$config['login']."\x01auth=Bearer ".system($config['xoauth2_cmd'])."\x01\x01");
		//print $config['xoauth2_enc_token']."\n";
	}
	if(isset($config['xoauth2_enc_token'])) {
		fwrite($fh, "x authenticate xoauth2 ".$config['xoauth2_enc_token']."\r\n");
	} 
	else if(isset($config['passwd'])) {
		fwrite($fh, "x login ".$config['login']." ".$config['passwd']."\r\n");
	}
	readl($fh);

	foreach($config['mboxes'] as $local=>$remote) {
		print "$local ($remote):\n";
		$srvlist=list_srv($fh, $config, $remote);
		print "on server : ".count($srvlist)."\n";
		$dblist=list_db($db, $config, $local);
		print "in db     : ".count(array_unique($dblist))."\n";
	//	print_r($dblist);
		$disklist=list_disk($config, $local);
		print "on disk   : ".count($disklist)."\n";
	//	print_r($disklist);
		prune_db($db, $config, $local, array_diff($dblist, $srvlist))."\n";
		prune_disk($config, $local, array_diff($disklist, $srvlist))."\n";
		file_put_contents($maildir."/".$config['cmAcc']."/".$local."/tofetch", join("\n", array_diff($srvlist, $disklist)));
	}
}

include "config.idler.php";
$cfg=$config[$_SERVER['argv'][1]];
$cmd=str_replace(array("__ACC","__MBOX"),array($cfg['cmAcc'],$cfg['cmMbox']),$cmd);
main_srv($cfg);

