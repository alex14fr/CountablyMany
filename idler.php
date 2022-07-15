<?php
function ts() {
	print "[".date("D M H:i:s")."] ";
}

function readl($fh) {
	stream_set_timeout($fh, 20);
	do {
		$l=fgets($fh);
		ts(); print "< $l";
	} while($l!==false && !feof($fh) && substr($l,0,2)!="x " && substr($l,0,2)!="+ ");
	ts(); print "---\n";
}

function mainloop($config) {
	global $cmd;
	while(true) {
		ts(); print "Connecting to ".$config['host']."... \n";
		$fh=stream_socket_client("tls://".$config['host']);
		if(!$fh) {
			ts();print "Error connecting\n";
			sleep(30);
		}
		if($config['xoauth2_cmd'] && !$config['xoauth2_enc_token']) {
			ts(); print "Fetching xoauth2 token...";
			$config['xoauth2_enc_token']=base64_encode("user=".$config['login']."\x01auth=Bearer ".system($config['xoauth2_cmd'])."\x01\x01");
			//print $config['xoauth2_enc_token']."\n";
		}
		if($config['xoauth2_enc_token']) {
			fwrite($fh, "x authenticate xoauth2 ".$config['xoauth2_enc_token']."\r\n");
		} 
		if($config['passwd']) {
			fwrite($fh, "x login ".$config['login']." ".$config['passwd']."\r\n");
		}
		readl($fh);
		$connectionok=true;
		while($connectionok) {
			fwrite($fh, "x select ".$config['cmMbox']."\r\n");
			readl($fh);
			stream_set_timeout($fh, 20*60);
			fwrite($fh, "x idle\r\n");
			readl($fh);
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
				if(!fwrite($fh, "done\r\n")) $connectionok=false;
				readl($fh);
				ts(); print "- Got $ln";
				if(strpos($ln,"EXISTS")!==false) {
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

