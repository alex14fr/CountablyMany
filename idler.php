<?php
function readl($fh) {
	do {
		$l=fgets($fh);
		print "< $l";
	} while(substr($l,0,2)!="x " && substr($l,0,2)!="+ ");
	print "---\n";
}

function mainloop($config) {
	global $cmd;
	while(true) {
		print "Connecting to ".$config['host']."... \n";
		$fh=stream_socket_client("tls://".$config['host']);
		if($config['xoauth2_cmd'] && !$config['xoauth2_enc_token']) {
			print "Fetching xoauth2 token...";
			$config['xoauth2_enc_token']=base64_encode("user=".$config['login']."\x01auth=Bearer ".system($config['xoauth2_cmd'])."\x01\x01");
			print $config['xoauth2_enc_token']."\n";
		}
		if($config['xoauth2_enc_token']) {
			fwrite($fh, "x authenticate xoauth2 ".$config['xoauth2_enc_token']."\r\n");
		} 
		if($config['passwd']) {
			fwrite($fh, "x login ".$config['login']." ".$config['passwd']."\r\n");
		}
		readl($fh);
		fwrite($fh, "x select inbox\r\n");
		readl($fh);
		$connectionok=true;
		while($connectionok) {
			fwrite($fh, "x idle\r\n");
			readl($fh);
			$ln=fgets($fh);
			if($ln===false || feof($fh)) {
				$connectionok=false;
			} else {
				if(!fwrite($fh, "done\r\n")) $connectionok=false;
				readl($fh);
				print "- Got $ln";
				if(strpos($ln,"EXISTS")!==false) {
					print "running cmd\n";
					system($cmd);
				}
				sleep(3);
			}
		}
	}
}

include "config.idler.php";

mainloop($config[$_SERVER['argv'][1]]);

