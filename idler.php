<?php
$connectionok=false;


function ts() {
	print "[".date("D M H:i:s")."] ";
}

function readl($fh) {
	global $connectionok;
	stream_set_timeout($fh, 20);
	do {
		$l=fgets($fh);
		ts(); print "< $l";
	} while($l!==false && !feof($fh) && substr($l,0,2)!="x " && substr($l,0,2)!="+ ");
	$timed_out=stream_get_meta_data($fh)['timed_out'];
	if($timed_out || /*($l===false) || */ feof($fh)) {
		print "readl(): error\n";
		$connectionok=false;
	}
	ts(); 
	print "---\n";
}

function mainloop($config) {
	global $cmd, $connectionok;
	while(true) {
		ts(); print "Connecting to ".$config['host']."... \n";
		$fh=stream_socket_client("tls://".$config['host']);
		if(!$fh) {
			ts();print "Error connecting\n";
			sleep(30);
			continue;
		}
		if(isset($config['xoauth2_cmd'])) {
			if(!isset($token_iat) || time()-$token_iat > 1900) {
				ts(); print "Fetching xoauth2 token...";
				$config['xoauth2_enc_token']=base64_encode("user=".$config['login']."\x01auth=Bearer ".system($config['xoauth2_cmd'])."\x01\x01");
				$token_iat=time();
				//print $config['xoauth2_enc_token']."\n";
			}
			fwrite($fh, "x authenticate xoauth2 ".$config['xoauth2_enc_token']."\r\n");
		}
		else { 
			fwrite($fh, "x login ".$config['login']." ".$config['passwd']."\r\n");
		}
		readl($fh);
		$connectionok=true;
		while($connectionok) {
			fwrite($fh, "x select ".$config['cmMbox']."\r\n");
			readl($fh);
			fwrite($fh, "x idle\r\n");
			readl($fh);
			stream_set_timeout($fh, 20*60);
			$ln=fgets($fh);
			$timed_out=stream_get_meta_data($fh)['timed_out'];
			if(!$timed_out && ($ln===false || feof($fh))) {
				$connectionok=false;
			} else if($timed_out) {
				ts(); print "- try reset idle\n";
				stream_set_timeout($fh, 20);
				if(!fwrite($fh, "done\r\n")) $connectionok=false;
				readl($fh);
			} else {
				ts(); print "-! Got $ln";
				if(!fwrite($fh, "done\r\n")) $connectionok=false;
				readl($fh);
				if(strpos($ln,"RECENT")!==false || strpos($ln,"EXISTS")!==false) {
					print "system($cmd)\n";
					system($cmd);
				}
				sleep(3);
			}
		}
	}
}

include "config.idler.php";
$cfg=$config[$_SERVER['argv'][1]];
$cmd=str_replace(array("__ACC","__MBOX"),array($cfg['cmAcc'],$cfg['cmMbox']),$cmd);
mainloop($cfg);

