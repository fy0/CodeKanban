package h

import (
	"fmt"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/validation"
)

var (
	msgUnexpectedProperty                 = "意外的属性"
	msgExpectedRFC3339DateTime            = "期望字符串为 RFC 3339 日期时间格式"
	msgExpectedRFC1123DateTime            = "期望字符串为 RFC 1123 日期时间格式"
	msgExpectedRFC3339Date                = "期望字符串为 RFC 3339 日期格式"
	msgExpectedRFC3339Time                = "期望字符串为 RFC 3339 时间格式"
	msgExpectedRFC5322Email               = "期望字符串为 RFC 5322 邮箱格式: %v"
	msgExpectedRFC5890Hostname            = "期望字符串为 RFC 5890 主机名格式"
	msgExpectedRFC2673IPv4                = "期望字符串为 RFC 2673 IPv4 格式"
	msgExpectedRFC2373IPv6                = "期望字符串为 RFC 2373 IPv6 格式"
	msgExpectedRFC3986URI                 = "期望字符串为 RFC 3986 URI 格式: %v"
	msgExpectedRFC4122UUID                = "期望字符串为 RFC 4122 UUID 格式: %v"
	msgExpectedRFC6570URITemplate         = "期望字符串为 RFC 6570 URI 模板格式"
	msgExpectedRFC6901JSONPointer         = "期望字符串为 RFC 6901 JSON 指针格式"
	msgExpectedRFC6901RelativeJSONPointer = "期望字符串为 RFC 6901 相对 JSON 指针格式"
	msgExpectedRegexp                     = "期望字符串为正则表达式格式: %v"
	msgExpectedMatchAtLeastOneSchema      = "期望值至少匹配一个模式，但没有匹配任何模式"
	msgExpectedMatchExactlyOneSchema      = "期望值恰好匹配一个模式，但没有匹配任何模式"
	msgExpectedNotMatchSchema             = "期望值不匹配模式"
	msgExpectedPropertyNameInObject       = "期望属性名值在对象中存在"
	msgExpectedBoolean                    = "期望布尔值"
	msgExpectedNumber                     = "期望数字"
	msgExpectedInteger                    = "期望整数"
	msgExpectedString                     = "期望字符串"
	msgExpectedBase64String               = "期望字符串为 Base64 编码格式"
	msgExpectedArray                      = "期望数组"
	msgExpectedObject                     = "期望对象"
	msgExpectedArrayItemsUnique           = "期望数组项目唯一"
	msgExpectedOneOf                      = "期望值为以下之一：\"%s\""
	msgExpectedMinimumNumber              = "期望数字 >= %v"
	msgExpectedExclusiveMinimumNumber     = "期望数字 > %v"
	msgExpectedMaximumNumber              = "期望数字 <= %v"
	msgExpectedExclusiveMaximumNumber     = "期望数字 < %v"
	msgExpectedNumberBeMultipleOf         = "期望数字为 %v 的倍数"
	msgExpectedMinLength                  = "期望长度 >= %d"
	msgExpectedMaxLength                  = "期望长度 <= %d"
	msgExpectedBePattern                  = "期望字符串为 %s"
	msgExpectedMatchPattern               = "期望字符串匹配模式 %s"
	msgExpectedMinItems                   = "期望数组长度 >= %d"
	msgExpectedMaxItems                   = "期望数组长度 <= %d"
	msgExpectedMinProperties              = "期望对象至少有 %d 个属性"
	msgExpectedMaxProperties              = "期望对象最多有 %d 个属性"
	msgExpectedRequiredProperty           = "期望必需属性 %s 存在"
	msgExpectedDependentRequiredProperty  = "当 %s 存在时，期望属性 %s 存在"
)

func HumaValidatePatch() {
	huma.ErrorFormatter = func(format string, a ...any) string {
		// 映射所有 validation 包的错误消息到中文
		switch format {
		case validation.MsgUnexpectedProperty:
			return msgUnexpectedProperty
		case validation.MsgExpectedRFC3339DateTime:
			return msgExpectedRFC3339DateTime
		case validation.MsgExpectedRFC1123DateTime:
			return msgExpectedRFC1123DateTime
		case validation.MsgExpectedRFC3339Date:
			return msgExpectedRFC3339Date
		case validation.MsgExpectedRFC3339Time:
			return msgExpectedRFC3339Time
		case validation.MsgExpectedRFC5322Email:
			return fmt.Sprintf(msgExpectedRFC5322Email, a...)
		case validation.MsgExpectedRFC5890Hostname:
			return msgExpectedRFC5890Hostname
		case validation.MsgExpectedRFC2673IPv4:
			return msgExpectedRFC2673IPv4
		case validation.MsgExpectedRFC2373IPv6:
			return msgExpectedRFC2373IPv6
		case validation.MsgExpectedRFC3986URI:
			return fmt.Sprintf(msgExpectedRFC3986URI, a...)
		case validation.MsgExpectedRFC4122UUID:
			return fmt.Sprintf(msgExpectedRFC4122UUID, a...)
		case validation.MsgExpectedRFC6570URITemplate:
			return msgExpectedRFC6570URITemplate
		case validation.MsgExpectedRFC6901JSONPointer:
			return msgExpectedRFC6901JSONPointer
		case validation.MsgExpectedRFC6901RelativeJSONPointer:
			return msgExpectedRFC6901RelativeJSONPointer
		case validation.MsgExpectedRegexp:
			return fmt.Sprintf(msgExpectedRegexp, a...)
		case validation.MsgExpectedMatchAtLeastOneSchema:
			return msgExpectedMatchAtLeastOneSchema
		case validation.MsgExpectedMatchExactlyOneSchema:
			return msgExpectedMatchExactlyOneSchema
		case validation.MsgExpectedNotMatchSchema:
			return msgExpectedNotMatchSchema
		case validation.MsgExpectedPropertyNameInObject:
			return msgExpectedPropertyNameInObject
		case validation.MsgExpectedBoolean:
			return msgExpectedBoolean
		case validation.MsgExpectedNumber:
			return msgExpectedNumber
		case validation.MsgExpectedInteger:
			return msgExpectedInteger
		case validation.MsgExpectedString:
			return msgExpectedString
		case validation.MsgExpectedBase64String:
			return msgExpectedBase64String
		case validation.MsgExpectedArray:
			return msgExpectedArray
		case validation.MsgExpectedObject:
			return msgExpectedObject
		case validation.MsgExpectedArrayItemsUnique:
			return msgExpectedArrayItemsUnique
		case validation.MsgExpectedOneOf:
			return fmt.Sprintf(msgExpectedOneOf, a...)
		case validation.MsgExpectedMinimumNumber:
			return fmt.Sprintf(msgExpectedMinimumNumber, a...)
		case validation.MsgExpectedExclusiveMinimumNumber:
			return fmt.Sprintf(msgExpectedExclusiveMinimumNumber, a...)
		case validation.MsgExpectedMaximumNumber:
			return fmt.Sprintf(msgExpectedMaximumNumber, a...)
		case validation.MsgExpectedExclusiveMaximumNumber:
			return fmt.Sprintf(msgExpectedExclusiveMaximumNumber, a...)
		case validation.MsgExpectedNumberBeMultipleOf:
			return fmt.Sprintf(msgExpectedNumberBeMultipleOf, a...)
		case validation.MsgExpectedMinLength:
			return fmt.Sprintf(msgExpectedMinLength, a...)
		case validation.MsgExpectedMaxLength:
			return fmt.Sprintf(msgExpectedMaxLength, a...)
		case validation.MsgExpectedBePattern:
			return fmt.Sprintf(msgExpectedBePattern, a...)
		case validation.MsgExpectedMatchPattern:
			return fmt.Sprintf(msgExpectedMatchPattern, a...)
		case validation.MsgExpectedMinItems:
			return fmt.Sprintf(msgExpectedMinItems, a...)
		case validation.MsgExpectedMaxItems:
			return fmt.Sprintf(msgExpectedMaxItems, a...)
		case validation.MsgExpectedMinProperties:
			return fmt.Sprintf(msgExpectedMinProperties, a...)
		case validation.MsgExpectedMaxProperties:
			return fmt.Sprintf(msgExpectedMaxProperties, a...)
		case validation.MsgExpectedRequiredProperty:
			return fmt.Sprintf(msgExpectedRequiredProperty, a...)
		case validation.MsgExpectedDependentRequiredProperty:
			return fmt.Sprintf(msgExpectedDependentRequiredProperty, a...)
		default:
			// 如果没有匹配的中文消息，返回格式化后的原始消息
			return fmt.Sprintf(format, a...)
		}
	}
}
