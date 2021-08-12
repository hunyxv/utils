package emailnotify

import "io"

// MessageImplementer 模板接口，使用者自己实现
type MessageImplementer interface {
	Content(io.Writer) error
}
