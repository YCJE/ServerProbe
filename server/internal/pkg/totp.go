package pkg

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// totpPeriod TOTP 时间步长（秒），RFC 6238 推荐 30 秒
const totpPeriod = 30

// totpDigits 验证码位数，Google Authenticator 等主流 App 使用 6 位
const totpDigits = 6

// GenerateTOTPSecret 生成随机 TOTP 密钥（20 字节，base32 编码）
func GenerateTOTPSecret() (string, error) {
	buf := make([]byte, 20)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("生成随机密钥失败: %w", err)
	}
	// 去除 padding，Google Authenticator 兼容无填充格式
	return strings.TrimRight(base32.StdEncoding.EncodeToString(buf), "="), nil
}

// GenerateOTPAuthURL 生成 otpauth:// URL，供认证器 App 扫码导入
func GenerateOTPAuthURL(secret, account, issuer string) string {
	return fmt.Sprintf(
		"otpauth://totp/%s:%s?secret=%s&issuer=%s&algorithm=SHA1&digits=%d&period=%d",
		url.PathEscape(issuer), url.PathEscape(account), secret, url.QueryEscape(issuer),
		totpDigits, totpPeriod,
	)
}

// totpCodeAt 计算指定时间步的 TOTP 码（HMAC-SHA1，RFC 4226 动态截断）
func totpCodeAt(secret []byte, step int64) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step))

	mac := hmac.New(sha1.New, secret)
	mac.Write(msg[:])
	sum := mac.Sum(nil)

	// 动态截断：取最后一个字节的低 4 位作为偏移
	offset := sum[len(sum)-1] & 0x0f
	code := (binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff) % 1000000
	return fmt.Sprintf("%06d", code)
}

// ValidateTOTP 验证 TOTP 验证码，允许 ±1 个时间步的时钟偏差
// 恒定时间比较防止时序攻击
func ValidateTOTP(secret, code string) bool {
	ok, _ := ValidateTOTPWithStep(secret, code)
	return ok
}

// ValidateTOTPWithStep 验证 TOTP 验证码并返回匹配的时间步（未匹配返回 0）
// 返回的 step 供调用方实现 RFC 6238 §5.2 要求的一次性使用（防重放）
func ValidateTOTPWithStep(secret, code string) (bool, int64) {
	code = strings.TrimSpace(code)
	if len(code) != totpDigits {
		return false, 0
	}
	// base32 补齐 padding
	padded := strings.ToUpper(secret)
	if m := len(padded) % 8; m != 0 {
		padded += strings.Repeat("=", 8-m)
	}
	key, err := base32.StdEncoding.DecodeString(padded)
	if err != nil {
		return false, 0
	}

	now := time.Now().Unix() / totpPeriod
	for _, delta := range []int64{0, -1, 1} {
		step := now + delta
		expected := totpCodeAt(key, step)
		if subtle.ConstantTimeCompare([]byte(expected), []byte(code)) == 1 {
			return true, step
		}
	}
	return false, 0
}
