<?php
if($argc==1) {
	echo 'Usage : ', $argv[0], ' [Mail folder] [SQL where] [Destination]', "\n";
	exit;
}
$dir=$argv[1];
$sql=$argv[2];
$dest=$argv[3];
$db=new SQLite3($dir."/Index.sqlite");
$res=$db->query("select * from messages where $sql");
while($a=$res->fetchArray(SQLITE3_ASSOC)) {
	print "# ".$a['f']." - ".$a['s']." - ".$a['d']."\n";
	if($dest=='KILL')
		print "rm $dir/".$a['a']."/".$a['m']."/".$a['u']."\n";
	else
		print "mv $dir/".$a['a']."/".$a['m']."/".$a['u']." $dir/".$a['a']."/$dest\n";
	print "echo $dest > $dir/".$a['a']."/".$a['m']."/moves/".$a['u']."\n";
}
