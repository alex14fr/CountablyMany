<?php
include "config.idler.php";
$db=new SQLite3($maildir."/Index.sqlite");
$res=$db->query("select rowid,* from messages order by rowid desc");
$db->exec("begin transaction");
while($a=$res->fetchArray()) {
	/*
	echo $a['rowid'],' ',$a['u'],' ',$a['a'],' ',$a['m'],": \n";
	$res2=$db->query("select rowid,* from messages where rowid<".$a['rowid']." and u=".$a['u']." and a='".$a['a']."' and m='".$a['m']."'");
	while($b=$res2->fetchArray())
		echo '   --> ', $b['rowid'],' ',$b['u'],' ',$b['a'],' ',$b['m'],"\n";
	*/
	$db->exec("delete from messages where rowid<".$a['rowid']." and u=".$a['u']." and a='".$a['a']."' and m='".$a['m']."'");
}
$db->exec("commit; vacuum; ");

