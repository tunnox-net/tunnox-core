package constants

// HTTP状态码常量
const (
	HTTPStatusOK                  = 200
	HTTPStatusCreated             = 201
	HTTPStatusNoContent           = 204
	HTTPStatusBadRequest          = 400
	HTTPStatusUnauthorized        = 401
	HTTPStatusForbidden           = 403
	HTTPStatusNotFound            = 404
	HTTPStatusMethodNotAllowed    = 405
	HTTPStatusConflict            = 409
	HTTPStatusUnprocessableEntity = 422
	HTTPStatusTooManyRequests     = 429
	HTTPStatusInternalServerError = 500
	HTTPStatusServiceUnavailable  = 503
)

// HTTP方法常量
const (
	HTTPMethodGET    = "GET"
	HTTPMethodPOST   = "POST"
	HTTPMethodPUT    = "PUT"
	HTTPMethodDELETE = "DELETE"
	HTTPMethodPATCH  = "PATCH"
)

// HTTP头部常量
const (
	HTTPHeaderContentType    = "Content-Type"
	HTTPHeaderAuthorization  = "Authorization"
	HTTPHeaderUserAgent      = "User-Agent"
	HTTPHeaderXRequestID     = "X-Request-ID"
	HTTPHeaderXForwardedFor  = "X-Forwarded-For"
	HTTPHeaderXRealIP        = "X-Real-IP"
	HTTPHeaderAccept         = "Accept"
	HTTPHeaderAcceptEncoding = "Accept-Encoding"
	HTTPHeaderCacheControl   = "Cache-Control"
	HTTPHeaderETag           = "ETag"
	HTTPHeaderLastModified   = "Last-Modified"
)

// Content-Type常量
const (
	ContentTypeJSON           = "application/json"
	ContentTypeXML            = "application/xml"
	ContentTypeFormURLEncoded = "application/x-www-form-urlencoded"
	ContentTypeMultipartForm  = "multipart/form-data"
	ContentTypeTextPlain      = "text/plain"
	ContentTypeTextHTML       = "text/html"
	ContentTypeOctetStream    = "application/octet-stream"
)

// 响应消息常量
const (
	ResponseMsgSuccess            = "Success"
	ResponseMsgCreated            = "Created successfully"
	ResponseMsgUpdated            = "Updated successfully"
	ResponseMsgDeleted            = "Deleted successfully"
	ResponseMsgNotFound           = "Resource not found"
	ResponseMsgUnauthorized       = "Unauthorized"
	ResponseMsgForbidden          = "Forbidden"
	ResponseMsgBadRequest         = "Bad request"
	ResponseMsgValidationFailed   = "Validation failed"
	ResponseMsgInternalError      = "Internal server error"
	ResponseMsgServiceUnavailable = "Service unavailable"
	ResponseMsgTooManyRequests    = "Too many requests"
)

// API路径常量
const (
	APIPathHealth    = "/health"
	APIPathMetrics   = "/metrics"
	APIPathAPI       = "/api"
	APIPathV1        = "/api/v1"
	APIPathAuth      = "/api/v1/auth"
	APIPathUsers     = "/api/v1/users"
	APIPathClients   = "/api/v1/clients"
	APIPathNodes     = "/api/v1/nodes"
	APIPathMappings  = "/api/v1/mappings"
	APIPathStats     = "/api/v1/stats"
	APIPathAnonymous = "/api/v1/anonymous"
)

// 分页常量
const (
	DefaultPageSize = 20
	MaxPageSize     = 100
	DefaultPage     = 1
)

// 排序常量
const (
	SortOrderASC  = "asc"
	SortOrderDESC = "desc"
)

// 时间格式常量
const (
	TimeFormatRFC3339 = "2006-01-02T15:04:05Z07:00"
	TimeFormatISO8601 = "2006-01-02T15:04:05.000Z"
	TimeFormatDate    = "2006-01-02"
	TimeFormatTime    = "15:04:05"
)
