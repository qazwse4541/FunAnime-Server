package serviceUser

import (
	"github.com/jinzhu/gorm"
	"github.com/mervick/aes-everywhere/go/aes256"
	"github.com/spf13/viper"
	"math/rand"
	"sinblog.cn/FunAnime-Server/cache"
	"sinblog.cn/FunAnime-Server/middleware/token"
	"sinblog.cn/FunAnime-Server/model"
	"sinblog.cn/FunAnime-Server/serializable/request/user"
	serviceCommon "sinblog.cn/FunAnime-Server/service/common"
	"sinblog.cn/FunAnime-Server/util/consts"
	"sinblog.cn/FunAnime-Server/util/errno"
	"sinblog.cn/FunAnime-Server/util/random"
	"strconv"
	"time"
)

func RegisterUser(userRequest *user.RegisterRequestInfo) int64 {
	_, userCount, err := model.QueryUserWithWhereMap(
		map[string]interface{}{
			"phone": userRequest.Phone,
		},
		map[string]interface{}{
			"status <> ?": model.UserDeleted,
		},
	)

	if err != nil && err != gorm.ErrRecordNotFound {
		return errno.DBOpError
	}

	if userCount != 0 && err != gorm.ErrRecordNotFound {
		return errno.PhoneHasResisted
	}

	flag, err := checkSmsCodeSuccess(userRequest.Phone, userRequest.SmsCode, user.Register)
	if err != nil {
		return errno.SmsCodeNotSend
	}

	if !flag {
		return errno.SmsCodeNotRight
	}

	_, err = model.CreateUserWithInstance(&model.User{
		Username:   random.GenEncryptUserName(userRequest.Phone),
		Nickname:   random.GenEncryptUserName(userRequest.Phone),
		Password:   aes256.Encrypt(random.GenRandomPassword(), viper.GetString("secret_key.password_key")),
		Phone:      userRequest.Phone,
		Sex:        model.NotCommit,
		Level:      0,
		ExpCount:   0,
		Status:     model.UserNotActive,
		Birthday:   consts.ZeroTime,
		CreateTime: time.Now(),
		ModifyTime: time.Now(),
	})
	if err != nil {
		return errno.DBOpError
	}

	return errno.Success
}

func checkSmsCodeSuccess(phone, smsCode string, smsType int) (bool, error) {
	sCode, err := cache.GetSmsCode(phone, smsType)
	if err != nil || sCode == "" {
		return false, err
	}
	if sCode != smsCode {
		return false, nil
	}
	return true, nil
}

func SendSmsCode(request *user.SendSmsRequest) error {
	smsCode := random.GenValidateCode()

	randTime := rand.Intn(3)
	minute := 15
	expireTime := time.Minute*time.Duration(minute) + time.Second*time.Duration(randTime)

	err := cache.SetSmsCode(request.Phone, request.Type, smsCode, expireTime)
	if err != nil {
		return err
	}

	// 发送短信
	err = serviceCommon.SendSms(request.Phone, smsCode, strconv.Itoa(minute))
	if err != nil {
		return err
	}

	return nil
}

func checkPasswordRight(requestPassword, dbPassword string) bool {
	return requestPassword == aes256.Decrypt(dbPassword, viper.GetString("secret_key.password_key"))
}

func LoginUser(userRequest *user.LoginRequestInfo) (string, int64) {
	userList, userCount, err := model.QueryUserWithWhereMap(
		map[string]interface{}{
			"phone": userRequest.Phone,
		},
		map[string]interface{}{
			"status <> ?": model.UserDeleted,
		},
	)

	if err != nil && err != gorm.ErrRecordNotFound {
		return "", errno.DBOpError
	}

	if err == gorm.ErrRecordNotFound || userCount == 0 || len(userList) <= 0 {
		return "", errno.PhoneNotExistence
	}

	userInfo := userList[0]
	flag := false
	if userRequest.Password != "" {
		flag = checkPasswordRight(userRequest.Password, userInfo.Password)
	} else if userRequest.SmsCode != "" {
		flag, err = checkSmsCodeSuccess(userRequest.Phone, userRequest.SmsCode, user.Login)
		if err != nil {
			return "", errno.SmsCodeNotSend
		}
	}

	if !flag {
		return "", errno.LoginInfoFailed
	}

	tokenUserInfo := &token.UserInfo{
		UserId:   userInfo.Id,
		Level:    userInfo.Level,
		Phone:    userInfo.Phone,
		Nickname: userInfo.Nickname,
		Username: userInfo.Username,
		Exp:      userInfo.ExpCount,
		Sex:      userInfo.Sex,
	}

	tToken, err := token.NewJWT().CreateToken(tokenUserInfo)
	if err != nil {
		return "", errno.TokenInvalid
	}
	return tToken, errno.Success
}
