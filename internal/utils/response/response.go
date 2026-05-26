package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

type PageResult struct {
	List  interface{} `json:"list"`
	Total int64       `json:"total"`
}

func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code:    0,
		Message: "success",
		Data:    data,
	})
}

func SuccessWithMessage(c *gin.Context, message string, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code:    0,
		Message: message,
		Data:    data,
	})
}

func Error(c *gin.Context, code int, message string) {
	c.JSON(http.StatusOK, Response{
		Code:    code,
		Message: message,
	})
}

func ErrorWithCode(c *gin.Context, httpCode int, code int, message string) {
	c.JSON(httpCode, Response{
		Code:    code,
		Message: message,
	})
}

func Page(c *gin.Context, list interface{}, total int64) {
	Success(c, PageResult{
		List:  list,
		Total: total,
	})
}

const (
	CodeSuccess          = 0
	CodeError            = 400
	CodeUnauthorized     = 401
	CodeForbidden        = 403
	CodeNotFound         = 404
	CodeServerError      = 500
	CodeInvalidParam     = 4001
	CodeInvalidUser      = 4002
	CodeInvalidPassword  = 4003
	CodeTokenExpired     = 4004
	CodeTokenInvalid     = 4005
	CodePermissionDenied = 4006
	CodeResourceExists   = 4007
)
