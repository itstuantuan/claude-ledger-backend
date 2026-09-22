package validation

import (
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

func New() *validator.Validate {
	v := validator.New()
	v.RegisterTagNameFunc(func(field reflect.StructField) string {
		name := strings.SplitN(field.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		if name == "" {
			return field.Name
		}
		return name
	})
	return v
}

func FieldErrors(err error) map[string][]string {
	result := map[string][]string{}
	if errors, ok := err.(validator.ValidationErrors); ok {
		for _, item := range errors {
			result[item.Field()] = append(result[item.Field()], "字段校验失败："+item.Tag())
		}
	}
	return result
}
