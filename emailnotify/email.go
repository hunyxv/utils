package emailnotify

import (
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/smtp"
	"regexp"
	"strings"
	"sync"
	"time"
)

var (
	// email 地址正则表达式
	p = regexp.MustCompile(`^([\w]+\.*)([\w]+)\@[\w]+\.\w{3}(\.\w{2}|)$`)

	Err_Incorrect = errors.New("incorrect email address format")
)

type emailAddrs struct {
	addrs []struct {
		nick, eaddr string
	}
	mutex sync.RWMutex
}

func (eaddr *emailAddrs) Add(nick, emailaddr string) error {
	if !p.Match([]byte(emailaddr)) {
		return Err_Incorrect
	}
	eaddr.mutex.Lock()
	defer eaddr.mutex.Unlock()

	eaddr.addrs = append(eaddr.addrs, struct {
		nick  string
		eaddr string
	}{nick: nick, eaddr: emailaddr})
	return nil
}

func (eaddr *emailAddrs) Len() int {
	eaddr.mutex.RLock()
	defer eaddr.mutex.RUnlock()
	return len(eaddr.addrs)
}

func (eaddr *emailAddrs) DeleteByEmail(email string) {
	eaddr.mutex.Lock()
	defer eaddr.mutex.Unlock()
	for i, addr := range eaddr.addrs {
		if addr.eaddr == email {
			eaddr.addrs = append(eaddr.addrs[:i], eaddr.addrs[i+1:]...)
			break
		}
	}
}

func (eaddr *emailAddrs) Flushall() {
	eaddr.mutex.Lock()
	defer eaddr.mutex.Unlock()
	eaddr.addrs = eaddr.addrs[:0]
}

func (eaddr *emailAddrs) DeleteByNick(nick string) {
	eaddr.mutex.Lock()
	defer eaddr.mutex.Unlock()
	for i, addr := range eaddr.addrs {
		if addr.nick == nick {
			eaddr.addrs = append(eaddr.addrs[:i], eaddr.addrs[i+1:]...)
		}
	}
}

func (eaddr *emailAddrs) AddrList() []string {
	eaddr.mutex.RLock()
	defer eaddr.mutex.RUnlock()
	lto := make([]string, len(eaddr.addrs))
	for i, addr := range eaddr.addrs {
		lto[i] = addr.eaddr
	}
	return lto
}

func (eaddr *emailAddrs) String() string {
	eaddr.mutex.RLock()
	defer eaddr.mutex.RUnlock()
	lto := make([]string, len(eaddr.addrs))
	for i, addr := range eaddr.addrs {
		lto[i] = fmt.Sprintf("%s<%s>", addr.nick, addr.eaddr)
	}
	return strings.Join(lto, ",")
}

// Mail .
type Mail struct {
	Nickname string
	From     string
	To       *emailAddrs
	Cc       *emailAddrs
	Host     string
	Port     int

	auth smtp.Auth
}

// NewMail 创建Mail
func NewMail(options ...Option) *Mail {
	opt := &option{
		Nickname: "Notify",
		SMTPPort: 25,
	}
	for _, f := range options {
		f(opt)
	}

	if opt.EmailFrom == "" {
		WithEmailFrom(opt.AccountUserName)(opt)
	}

	return &Mail{
		Nickname: opt.Nickname,
		From:     opt.EmailFrom,
		To:       &emailAddrs{},
		Cc:       &emailAddrs{},
		Host:     opt.SMTPHost,
		Port:     opt.SMTPPort,

		auth: smtp.PlainAuth("", opt.AccountUserName, opt.AccountPasswd, opt.SMTPHost),
	}
}

func (m *Mail) dial(addr string) (*smtp.Client, error) {
	conn, err := tls.Dial("tcp", addr, nil)
	if err != nil {
		return nil, err
	}

	host, _, _ := net.SplitHostPort(addr)
	return smtp.NewClient(conn, host)
}

func (m *Mail) writeHeader(w io.Writer, header map[string]string) error {
	h := &bytes.Buffer{}
	for k, v := range header {
		h.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
	}
	h.WriteString("\r\n")

	_, err := w.Write(h.Bytes())
	if err != nil {
		return err
	}
	return nil
}

// Send 根据 模板发送邮件
func (m *Mail) Send(subject string, content MessageImplementer) (err error) {
	if m.To.Len() == 0 {
		return
	}

	c, err := m.dial(fmt.Sprintf("%s:%d", m.Host, m.Port))
	if err != nil {
		return err
	}

	if ok, _ := c.Extension("AUTH"); ok {
		if err = c.Auth(m.auth); err != nil {
			return err
		}
	}
	if err = c.Mail(m.From); err != nil {
		return err
	}

	for _, addr := range m.To.AddrList() {
		if err = c.Rcpt(addr); err != nil {
			return err
		}
	}

	for _, addr := range m.Cc.AddrList() {
		if err = c.Rcpt(addr); err != nil {
			return err
		}
	}

	w, err := c.Data()
	if err != nil {
		return err
	}
	defer w.Close()

	header := make(map[string]string)
	header["From"] = fmt.Sprintf("%s <%s>", m.Nickname, m.From)
	header["To"] = m.To.String()
	header["Cc"] = m.Cc.String()
	header["Subject"] = subject
	header["Content-Type"] = "text/html; charset=UTF-8"
	header["Date"] = time.Now().String()
	if err = m.writeHeader(w, header); err != nil {
		return err
	}

	err = content.Content(w)
	return err
}
