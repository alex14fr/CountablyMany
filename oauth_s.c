#include <sys/socket.h>
#include <netinet/in.h>
#include <string.h>
#include <stdio.h>
#include <fcntl.h>

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
	bind(s,&addr,sizeof(struct sockaddr_in));
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
	write(s2,OKMSG,strlen(OKMSG));
}
