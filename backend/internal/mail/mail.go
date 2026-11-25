package mail

import (
	"fmt"
	"log"
	"net/smtp"
	"strings"
	"time"
)

type Sender interface {
	Send(to, subject, text, html string) error
}

type SMTP struct {
	addr, from, user, pass string
}

type Log struct{}

func New(host, port, user, pass, from string) Sender {
	if host == "" {
		return Log{}
	}
	return &SMTP{addr: host + ":" + port, from: from, user: user, pass: pass}
}

func (s *SMTP) Send(to, subject, text, html string) error {
	if strings.ContainsAny(to+subject+s.from, "\r\n") {
		return fmt.Errorf("invalid header value")
	}
	const boundary = "vaultly-alt-boundary"
	msg := fmt.Sprintf("From: Vaultly <%s>\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=%s\r\n\r\n"+
		"--%s\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s\r\n--%s\r\nContent-Type: text/html; charset=utf-8\r\n\r\n%s\r\n--%s--\r\n",
		s.from, to, subject, time.Now().Format(time.RFC1123Z), boundary, boundary, text, boundary, html, boundary)
	var auth smtp.Auth
	if s.user != "" {
		host := strings.Split(s.addr, ":")[0]
		auth = smtp.PlainAuth("", s.user, s.pass, host)
	}
	return smtp.SendMail(s.addr, auth, s.from, []string{to}, []byte(msg))
}

func (Log) Send(to, subject, text, _ string) error {
	log.Printf("mail (SMTP_HOST not set, not sent) to=%s subject=%q\n%s", to, subject, text)
	return nil
}

var (
	_ Sender = (*SMTP)(nil)
	_ Sender = Log{}
)
