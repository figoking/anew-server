package middleware

import (
	"anew-server/dto/request"
	"anew-server/dto/response"
	"anew-server/dto/service"
	"anew-server/pkg/common"
	"anew-server/pkg/redis"
	"anew-server/pkg/utils"
	"fmt"
	jwt "github.com/appleboy/gin-jwt/v2"
	"github.com/gin-gonic/gin"
	"time"
)

// 创建一个userInfo全局变量来返回用户信息
var loginInfo response.LoginResp

func InitAuth() (*jwt.GinJWTMiddleware, error) {
	return jwt.New(&jwt.GinJWTMiddleware{
		Realm:           common.Conf.Jwt.Realm,                                 // jwt标识
		SigningAlgorithm: "HS512",
		Key:             []byte(common.Conf.Jwt.Key),                           // 服务端密钥
		Timeout:         time.Hour * time.Duration(common.Conf.Jwt.Timeout),    // token过期时间
		MaxRefresh:      time.Hour * time.Duration(common.Conf.Jwt.MaxRefresh), // token更新时间
		PayloadFunc:     payloadFunc,                                           // 有效载荷处理
		IdentityHandler: identityHandler,                                       // 解析Claims
		Authenticator:   login,                                                 // 校验token的正确性, 处理登录逻辑
		Authorizator:    authorizator,                                          // 用户登录校验成功处理
		Unauthorized:    unauthorized,                                          // 用户登录校验失败处理
		LoginResponse:   loginResponse,                                         // 登录成功后的响应
		LogoutResponse:  logoutResponse,                                        // 登出后的响应
		TokenLookup:     "header: Authorization, query: token",                 // 自动在这几个地方寻找请求中的token
		TokenHeadName:   "Bearer",                                              // header名称
		TimeFunc:        time.Now,
	})
}

func payloadFunc(data interface{}) jwt.MapClaims {
	if v, ok := data.(map[string]interface{}); ok {
		var user response.LoginResp
		// 将用户json转为结构体
		utils.JsonI2Struct(v["user"], &user)
		return jwt.MapClaims{
			jwt.IdentityKey: user.Id,
			"user":          v["user"],
		}
	}
	return jwt.MapClaims{}
}

func identityHandler(c *gin.Context) interface{} {
	claims := jwt.ExtractClaims(c)
	// 此处返回值类型map[string]interface{}与payloadFunc和authorizator的data类型必须一致, 否则会导致授权失败还不容易找到原因
	return map[string]interface{}{
		"IdentityKey": claims[jwt.IdentityKey],
		"user":        claims["user"],
	}
}

func login(c *gin.Context) (interface{}, error) {
	var req request.RegisterAndLoginReq
	// 请求json绑定
	_ = c.ShouldBindJSON(&req)
	// 创建服务
	s := service.New()
	// 密码校验
	user, err := s.LoginCheck(req.Username, req.Password)
	if err != nil {
		return nil, err
	}
	loginInfo = *user
	// 将用户以json格式写入, payloadFunc/authorizator会使用到
	ma := map[string]interface{}{
		"user": utils.Struct2Json(user),
	}
	return ma, nil
}

func authorizator(data interface{}, c *gin.Context) bool {
	if v, ok := data.(map[string]interface{}); ok {
		var user response.LoginResp
		// 将用户json转为结构体
		utils.JsonI2Struct(v["user"], &user)
		// 将用户保存到context, api调用时取数据方便
		c.Set("user", user)
		return true
	}
	return false
}

func unauthorized(c *gin.Context, code int, message string) {
	common.Log.Debug(fmt.Sprintf("JWT认证失败, 错误码%d, 错误信息%s", code, message))
	if message == response.LoginCheckErrorMsg {
		response.FailWithMsg(response.LoginCheckErrorMsg)
		return
	} else if message == response.UserForbiddenMsg {
		response.FailWithCode(response.UserForbidden)
		return
	}

	response.FailWithCode(response.Unauthorized)
}

func loginResponse(c *gin.Context, code int, token string, expires time.Time) {

	// 缓存token
	cache := redis.NewStringOperation()
	tokenKey := "token:" + loginInfo.Username
	expiresKey := "expires:" + loginInfo.Username
	cacheToken := cache.Get(tokenKey).Unwrap()
	cacheExpires:= cache.Get(expiresKey).Unwrap()
	if cacheToken == "" {
		cacheToken = token
		// 超时时间为配置文件设置的值
		cache.Set(tokenKey, cacheToken, redis.WithExpire(time.Hour*time.Duration(common.Conf.Jwt.Timeout)))
	}
	if cacheExpires == "" {
		cacheExpires = expires.Format("2006-01-02 15:04:05")
		// 超时时间为配置文件设置的值
		cache.Set(expiresKey, cacheExpires, redis.WithExpire(time.Hour*time.Duration(common.Conf.Jwt.Timeout)))
	}
	loginInfo.Token = cacheToken
	loginInfo.Expires = cacheExpires
	response.SuccessWithData(loginInfo)
}

func logoutResponse(c *gin.Context, code int) {
	response.Success()
}
