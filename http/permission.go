package http

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/ArtalkJS/ArtalkGo/config"
	"github.com/ArtalkJS/ArtalkGo/lib"
	"github.com/ArtalkJS/ArtalkGo/model"
	"github.com/golang-jwt/jwt"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/sirupsen/logrus"
)

var permCtx = context.Background()
var CommonJwtConfig middleware.JWTConfig

// jwtCustomClaims are custom claims extending default ones.
// See https://github.com/golang-jwt/jwt for more examples
type jwtCustomClaims struct {
	UserName  string         `json:"name"`
	UserEmail string         `json:"email"`
	UserType  model.UserType `json:"type"`
	jwt.StandardClaims
}

type Skipper func(echo.Context) bool
type ActionPermissionConf struct {
	Skipper Skipper
}

// 操作限制 中间件
func ActionPermission(conf ActionPermissionConf) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if conf.Skipper(c) {
				return next(c)
			}

			if !CheckIsAdminReq(c) && IsActionOverLimit(c) {
				if config.Instance.Debug {
					logrus.Debug("[操作限制] 次数: ", getActionCount(c), ", 最后时间：", getActionLastTime(c))
				}
				return RespError(c, "需要验证码", Map{
					"need_captcha": true,
					"img_data":     GetNewCaptchaImageBase64(c.RealIP()),
				})
			} else {
				return next(c)
			}
		}
	}
}

// 操作是否超过限制
func IsActionOverLimit(c echo.Context) bool {
	if config.Instance.Captcha.Always { // 总是需要验证码
		return true
	}

	if time.Since(getActionLastTime(c)).Seconds() <= float64(config.Instance.Captcha.ActionTimeout) { // 在时间内
		if getActionCount(c) >= config.Instance.Captcha.ActionLimit { // 操作次数超过
			RecordAction(c) // 记录操作
			return true
		}
	} else { // 不在时间内，超时
		ResetActionRecord(c) // 重置操作记录
	}

	return false
}

// 记录操作
func RecordAction(c echo.Context) {
	updateActionLastTime(c) // 更新最后操作时间
	addActionCount(c)       // 操作次数 +1
}

// 重置操作记录
func ResetActionRecord(c echo.Context) {
	ip := c.RealIP()

	lib.CACHE.Delete(permCtx, "action-time:"+ip)
	lib.CACHE.Delete(permCtx, "action-count:"+ip)
}

// 修改最后操作时间
func updateActionLastTime(c echo.Context) {
	curtTime := fmt.Sprintf("%v", time.Now().Unix())
	lib.CACHE.Set(permCtx, "action-time:"+c.RealIP(), []byte(curtTime), nil)
}

// 获取最后操作时间
func getActionLastTime(c echo.Context) time.Time {
	var timestamp int64
	if val, err := lib.CACHE.Get(permCtx, "action-time:"+c.RealIP()); err == nil {
		timestamp, _ = strconv.ParseInt(string(val.([]byte)), 10, 64)
	}
	tm := time.Unix(timestamp, 0)
	return tm
}

// 获取操作次数
func getActionCount(c echo.Context) int {
	count := 0
	if val, err := lib.CACHE.Get(permCtx, "action-count:"+c.RealIP()); err == nil {
		count, _ = strconv.Atoi(string(val.([]byte)))
	}

	return count
}

// 修改操作次数
func setActionCount(c echo.Context, num int) {
	lib.CACHE.Set(permCtx, "action-count:"+c.RealIP(), []byte(fmt.Sprintf("%d", num)), nil)
}

// 操作次数 +1
func addActionCount(c echo.Context) {
	setActionCount(c, getActionCount(c)+1)
}

func CheckIsAdminReq(c echo.Context) bool {
	isAdmin := false
	token := c.Param("token")
	if token == "" {
		token = c.Request().Header.Get("X-Auth-Token")
	}
	if token != "" {
		jwt, err := jwt.Parse(token, func(t *jwt.Token) (interface{}, error) {
			if t.Method.Alg() != CommonJwtConfig.SigningMethod {
				return nil, fmt.Errorf("unexpected jwt signing method=%v", t.Header["alg"])
			}

			return []byte(config.Instance.AppKey), nil // 密钥
		})

		if err == nil {
			isAdmin = CheckIsAdminByJwt(jwt)
		}
	}
	return isAdmin
}

func CheckIsAdminByJwt(jwt *jwt.Token) bool {
	claims := jwt.Claims.(*jwtCustomClaims)
	name := claims.UserName
	email := claims.UserEmail

	if claims.UserType != model.UserAdmin {
		return false
	}

	// check user from database
	user := FindUser(name, email)
	if user.IsEmpty() {
		return false
	}

	return user.Type == model.UserAdmin
}

// 中间件会创建一个 user context，通过中间件获取到的 jwt 判断
func CheckIsAdminByJwtMiddleware(c echo.Context) bool {
	jwt := c.Get("user").(*jwt.Token)

	return CheckIsAdminByJwt(jwt)
}