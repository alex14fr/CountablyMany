<?php
$config=array(
0=>array('host'=>'imap.gmail.com:993','login'=>'CHANGETHIS','xoauth2_cmd'=>'./OAuthRefresh','cmAcc'=>'gmail','cmMbox'=>'inbox'),
1=>array('host'=>'secondhost:993','login'=>'CHANGETHIS','passwd'=>'CHANGETHIS','cmAcc'=>'changethis','cmMbox'=>'inbox')
);
$cmd="curl 'http://local-login:localpassword@localhost:1336/resync?quickacc=__ACC&quickmbox=__MBOX'";

