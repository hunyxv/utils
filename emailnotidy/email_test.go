package emailnotify

import (
	"io"
	"testing"
	"text/template"
)

var _ MessageImplementer = (*TestTemplate)(nil)

type TestTemplate struct {
	Title    string
	Message  string
	template string
}

func NewTestTemplate(title, message string) *TestTemplate {
	return &TestTemplate{
		Title:   title,
		Message: message,
		template: `<html>
			<body>
				<h1> {{ .Title }} </h1>

				<p> {{ .Message }} </p>
			</body>
		</html>`,
	}
}

func (t *TestTemplate) Content(w io.Writer) error {
	tmpl, err := template.New("email").Parse(t.template)
	if err != nil {
		return err
	}

	return tmpl.Execute(w, t)
}

func TestSendEmail(t *testing.T) {
	email := NewMail(WithAccountUserName("hunyxv@qq.com"), WithNickname("hunyxv"),
		WithAccountPasswd("xxxxxxxxxxxxxxx"), WithSMTPHost("smtp.qq.com"),
		WithSMTPPort(465))
	if err := email.To.Add("hunyxv", "hunyxv@gmail.com"); err != nil {
		t.Fatal(err)
	}

	temp := NewTestTemplate("测试邮件", "这是个测试！")

	if err := email.Send("你有一封新邮件", temp); err != nil {
		t.Fatal(err)
	}
}
