package emailnotify

type Option func(opt *option)

type option struct {
	Nickname        string // 我的昵称（显示给对方的）
	EmailFrom       string // 发送邮件的邮箱
	AccountUserName string // 邮箱验证用户名
	AccountPasswd   string // 邮箱验证密码
	SMTPHost        string // smtp 服务器地址
	SMTPPort        int    // smtp 服务器端口（默认 25）
}

// WithNickname 我的昵称
func WithNickname(nickname string) Option {
	return func(opt *option) {
		opt.Nickname = nickname
	}
}

// WithEmailFrom 发送邮件的邮箱
func WithEmailFrom(email string) Option {
	if !p.Match([]byte(email)) {
		panic(Err_Incorrect)
	}

	return func(opt *option) {
		opt.EmailFrom = email
	}
}

// WithAccountUserName 邮箱验证用户名
func WithAccountUserName(username string) Option {
	return func(opt *option) {
		opt.AccountUserName = username
	}
}

// WithAccountPasswd 邮箱验证密码
func WithAccountPasswd(passwd string) Option {
	return func(opt *option) {
		opt.AccountPasswd = passwd
	}
}

// WithSMTPHost smtp 服务器地址
func WithSMTPHost(host string) Option {
	return func(opt *option) {
		opt.SMTPHost = host
	}
}

// WithSMTPPort smtp 服务器端口 （默认 25）
func WithSMTPPort(port int) Option {
	return func(opt *option) {
		opt.SMTPPort = port
	}
}
