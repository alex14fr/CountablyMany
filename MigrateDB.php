<?php
$db=yaml_parse_file("/home/al/Mail2/Index.yaml");
print "delete from messages;\n";
foreach($db as $it) {
	print "insert into messages (u,a,m,f,s,d,i) values(";
	foreach(array('u','a','m','f','s','d','i') as $k) {
		print "'".SQLite3::escapeString($it[$k]).($k=='i' ? "'" : "',");
	}
	print ");\n";

}
