package main

import (
	"bufio"
	"os"
	"strings"
)

var GlobConf map[string]string
var Mailboxes map[string](map[string]string)
var SMTPServ map[string](map[string]string)
var IMAPServ map[string](map[string]string)

func readConfig() {
	GlobConf = make(map[string]string)
	Mailboxes = make(map[string](map[string]string))
	SMTPServ = make(map[string](map[string]string))
	IMAPServ = make(map[string](map[string]string))
	f, err := os.Open("CountablyMany.ini")
	if err != nil {
		println("error reading conf file")
		os.Exit(1)
	}
	sc := bufio.NewReader(f)
	mode := 0
	acc := ""
	for true {
		line, err := sc.ReadString('\n')
		line = strings.Trim(line, "\r\n ")
		if len(line)>=1 && line[0]=='#' {
			continue
		}
		if strings.Index(line, "[all]") == 0 {
			mode = 1
		} else if strings.Index(line, "[") == 0 && strings.Index(line, ".imap]") >= 0 {
			mode = 2
		} else if strings.Index(line, "[") == 0 && strings.Index(line, ".smtp]") >= 0 {
			mode = 3
		}
		if mode != 1 && strings.Index(line, "[") == 0 {
			acc = line[1 : len(line)-6]
			if mode == 2 {
				IMAPServ[acc] = make(map[string]string)
			} else if mode == 3 {
				SMTPServ[acc] = make(map[string]string)
			}
		}
		if strings.Index(line, "[") != 0 {
			key, val, _ := strings.Cut(line, "=")
			if mode == 1 {
				GlobConf[key] = val
			} else if mode == 2 {
				IMAPServ[acc][key] = val
				if key == "Mailboxes" {
					Mailboxes[acc] = make(map[string]string)
					for _, mbxdef := range strings.Split(val, " ") {
						mbxdefsplt := strings.Split(mbxdef, "=")
						Mailboxes[acc][mbxdefsplt[0]] = mbxdefsplt[1]
					}
				}
			} else if mode == 3 {
				SMTPServ[acc][key] = val
			}
			if err != nil {
				break
			}
		}
	}
}
