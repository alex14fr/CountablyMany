//go:build ignore
#include <sys/socket.h>
#include <netinet/in.h>
#include <string.h>
#include <stdio.h>
#include <fcntl.h>
#include <unistd.h>
#include <stdlib.h>

#define PORT 16741
#define ERRMSG "HTTP/1.0 400 Bad request\r\n\r\n"
#define OKMSG "HTTP/1.0 200 Ok\r\nContent-type: text/html\r\n\r\nok"

void main(void) {
	struct sockaddr_in addr;
	int s=socket(AF_INET,SOCK_STREAM,0);
	setsockopt(s,SOL_SOCKET,SO_REUSEADDR,&(int){1},sizeof(int));
	int s2;
	char req[4096];
	addr.sin_family=AF_INET;
	addr.sin_port=htons(PORT);
	addr.sin_addr.s_addr=htonl(INADDR_LOOPBACK);
	bind(s,(struct sockaddr*)(&addr),sizeof(struct sockaddr_in));
	if(listen(s,1)<0) { perror("listen"); }
	s2=accept(s,NULL,0);
	int nread=0;
	nread=read(s2,req,4096);
	if(nread==4096) { write(s2,ERRMSG,strlen(ERRMSG)); close(s2); puts("request length exceeded"); }
	char *ss;
	ss=strchr(req,'\n');
	if(ss!=NULL) {
		*ss=0;
		puts(req);
	}
	char code[256];
	for(int i=0; i<strlen(req)-5; i++) {
		if(req[i]=='c' && req[i+1]=='o' && req[i+2]=='d' && req[i+3]=='e' && req[i+4]=='=') {
			int j;
			for(j=i+5; j<strlen(req) && req[j]!='&' && j-i-5<256; j++)
				if( (req[j]>='a' && req[j]<='z') || (req[j]>='A' && req[j]<='Z') || (req[j]>='0' && req[j]<='9') || req[j]=='/' || req[j]=='_' || req[j]=='-')
					code[j-i-5]=req[j];
				else
					puts("forbidden char in received code");
			code[j-i-5]=0;
		}
	}
	puts(code);
	write(s2,OKMSG,strlen(OKMSG));
	write(s2,code,strlen(code));
	char *cmd="./OAuthStep2";
	if(getenv("STEP2")) cmd=getenv("STEP2");
	char *argv[]={cmd, code, NULL};
	execve(cmd, argv, NULL);
}
