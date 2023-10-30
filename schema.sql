CREATE TABLE messages(u integer, a text, m text, f text, s text, d text, i text, t text, ut integer);
CREATE INDEX idx1 on messages (m,a);
CREATE INDEX idx2 on messages (i,a,m);
