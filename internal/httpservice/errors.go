package httpservice

import (
	coreerrors "tunnox-core/internal/core/errors"
)

// HTTP 服务相关错误码
const (
	// 域名代理错误
	CodeDomainNotFound     = coreerrors.CodeNotFound
	CodeClientOffline      = coreerrors.CodeUnavailable
	CodeProxyTimeout       = coreerrors.CodeTimeout
	CodeDomainAlreadyExist = coreerrors.CodeAlreadyExists
	CodeInvalidDomain      = coreerrors.CodeInvalidParam
	CodeBaseDomainNotAllow = coreerrors.CodeForbidden
)
