package account

import "errors"

// 账号域业务错误 sentinel（单一真源）。
//
// 目的：让 handler 层经 errors.Is 精准映射 HTTP 语义（而非一刀切 500/401 并回显
// err.Error()），同时对外文案安全可控、不泄漏内部实现（〔M7〕收尾，见〔R2-1〕）。
// 这些 sentinel 的 message 本身即为可直接展示给终端用户的安全文案。
var (
	// ErrEmailExists 注册邮箱在租户内已存在 → 409。
	ErrEmailExists = errors.New("邮箱已被注册")
	// ErrUsernameExists 注册用户名在租户内已存在 → 409。
	ErrUsernameExists = errors.New("用户名已被使用")
	// ErrInvalidCredential 登录凭据无效（邮箱不存在或密码错误，合并以防用户枚举）→ 401。
	ErrInvalidCredential = errors.New("邮箱或密码错误")
	// ErrAccountDisabled 账号被禁用 → 403。
	ErrAccountDisabled = errors.New("账号已被禁用")
	// ErrInvalidToken 令牌无效/类型错误/已失效（刷新流各失败分支归并）→ 401。
	ErrInvalidToken = errors.New("无效的令牌")
	// ErrUserNotFound 用户不存在 → 404。
	ErrUserNotFound = errors.New("用户不存在")
	// ErrOldPasswordIncorrect 修改密码时旧密码不匹配 → 400。
	ErrOldPasswordIncorrect = errors.New("旧密码错误")
)
