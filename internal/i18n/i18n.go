// Package i18n makes the API speak the interface language. The app is
// Russian-only today (owner 2026-07-28), so there is no negotiation and no
// catalog: Install() rewrites the two places whose text is NOT ours — huma's
// built-in validation messages and its error-model titles — and every message
// the modules produce themselves is simply written in Russian at its source.
//
// Both hooks are process-global huma variables, which is why they live in one
// explicit Install() called during assembly rather than in scattered init()s.
// A second interface language would replace this file with a real catalog +
// Accept-Language negotiation; the huma side would still be global, so it
// would need per-request formatting instead (see the ErrorFormatter hook).
package i18n

import (
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/validation"
)

// statusTitles replaces huma's http.StatusText() titles. Only the statuses
// this API actually returns are translated; anything else keeps the English
// status text, which is better than a wrong guess.
var statusTitles = map[int]string{
	http.StatusBadRequest:            "Некорректный запрос",
	http.StatusUnauthorized:          "Нужен вход",
	http.StatusForbidden:             "Нет доступа",
	http.StatusNotFound:              "Не найдено",
	http.StatusConflict:              "Конфликт",
	http.StatusRequestEntityTooLarge: "Файл слишком большой",
	http.StatusUnsupportedMediaType:  "Неподдерживаемый формат файла",
	http.StatusUnprocessableEntity:   "Не удалось обработать",
	http.StatusTooManyRequests:       "Слишком много запросов",
	http.StatusInternalServerError:   "Ошибка сервера",
	http.StatusServiceUnavailable:    "Сервис недоступен",
}

// humaDetails are the message strings huma itself passes to NewError (ours
// are already Russian at the call site). Keyed by huma's literal.
var humaDetails = map[string]string{
	"validation failed":          "проверка данных не прошла",
	"bad request":                "некорректный запрос",
	"request body is required":   "тело запроса обязательно",
	"unexpected error occurred":  "внутренняя ошибка",
	"unable to marshal response": "не удалось сформировать ответ",
}

// paramDetails are huma's parameter-parsing errors. They reach NewError as
// plain errors, so they arrive as detail messages rather than as the top
// message — same catalogue, different slot.
var paramDetails = map[string]string{
	"invalid integer":        "ожидается целое число",
	"invalid float":          "ожидается число",
	"invalid floating value": "ожидается число",
	"invalid boolean":        "ожидается да/нет",
	"invalid url.URL value":  "ожидается ссылка",
	"unparsable value":       "значение не разобрано",
	"unsupported type":       "неподдерживаемый тип значения",
}

// paramDetailPrefixes covers the parse errors huma builds by concatenation,
// where only the leading phrase is fixed.
var paramDetailPrefixes = [][2]string{
	{"invalid date/time for format ", "ожидается дата/время в формате "},
	{"invalid value: ", "некорректное значение: "},
	{"cannot read multipart form: ", "не удалось прочитать загруженный файл: "},
}

// uploadDetails are huma's multipart messages (attachment upload). Unlike the
// validation vocabulary these are literals inside huma, so they can only be
// caught here, on the way out.
var uploadDetails = map[string]string{
	"Failed to open file":                               "не удалось открыть файл",
	"Failed to infer file media type":                   "не удалось определить тип файла",
	"File required":                                     "нужно приложить файл",
	"At least one file is required":                     "нужно приложить хотя бы один файл",
	"Multiple files received but only one was expected": "ожидается один файл",
}

// mimeMismatch rebuilds huma's «Invalid mime type: got X, expected Y» — the
// one upload message carrying two interpolated values.
func mimeMismatch(msg string) (string, bool) {
	rest, found := strings.CutPrefix(msg, "Invalid mime type: got ")
	if !found {
		return "", false
	}
	got, want, ok := strings.Cut(rest, ", expected ")
	if !ok {
		return "", false
	}
	return "неподдерживаемый тип файла: " + got + ", допустимы: " + want, true
}

// translateDetail localizes one huma-produced detail message, leaving
// anything unrecognized (module messages, DB errors) untouched.
func translateDetail(msg string) string {
	if ru, ok := paramDetails[msg]; ok {
		return ru
	}
	if ru, ok := uploadDetails[msg]; ok {
		return ru
	}
	if ru, ok := mimeMismatch(msg); ok {
		return ru
	}
	for _, p := range paramDetailPrefixes {
		if after, found := strings.CutPrefix(msg, p[0]); found {
			return p[1] + after
		}
	}
	return msg
}

// StatusTitle is the Russian title for an HTTP status, falling back to Go's
// English status text.
func StatusTitle(status int) string {
	if t, ok := statusTitles[status]; ok {
		return t
	}
	return http.StatusText(status)
}

// Install switches huma to Russian. Safe to call more than once (tests build
// several servers); it only assigns package-level variables.
func Install() {
	installValidationMessages()

	// huma builds every error — ours via huma.Error4xx…(), its own via the
	// request validator — through this constructor, so translating the title
	// and its own detail strings here covers both. The body/details logic
	// mirrors huma's default implementation.
	huma.NewError = func(status int, msg string, errs ...error) huma.StatusError {
		details := make([]*huma.ErrorDetail, len(errs))
		for i := range errs {
			if converted, ok := errs[i].(huma.ErrorDetailer); ok {
				d := converted.ErrorDetail()
				d.Message = translateDetail(d.Message)
				details[i] = d
				continue
			}
			if errs[i] == nil {
				continue
			}
			details[i] = &huma.ErrorDetail{Message: translateDetail(errs[i].Error())}
		}
		if ru, ok := humaDetails[msg]; ok {
			msg = ru
		}
		return &huma.ErrorModel{
			Status: status,
			Title:  StatusTitle(status),
			Detail: msg,
			Errors: details,
		}
	}
}

// installValidationMessages translates huma's schema-validation vocabulary —
// the messages a user sees when a field is missing, mistyped or out of range.
// Format verbs are kept in the same order as upstream (they are positional).
func installValidationMessages() {
	validation.MsgUnexpectedProperty = "лишнее поле"
	validation.MsgExpectedRequiredProperty = "не заполнено обязательное поле %s"
	validation.MsgExpectedDependentRequiredProperty = "поле %s обязательно, когда заполнено %s"

	// Types.
	validation.MsgExpectedBoolean = "ожидается да/нет"
	validation.MsgExpectedNumber = "ожидается число"
	validation.MsgExpectedInteger = "ожидается целое число"
	validation.MsgExpectedString = "ожидается строка"
	validation.MsgExpectedArray = "ожидается список"
	validation.MsgExpectedObject = "ожидается объект"
	validation.MsgExpectedBase64String = "ожидается строка в base64"
	validation.MsgExpectedDuration = "ожидается длительность: %v"

	// Formats.
	validation.MsgExpectedRFC3339DateTime = "ожидается дата и время в формате RFC 3339"
	validation.MsgExpectedRFC1123DateTime = "ожидается дата и время в формате RFC 1123"
	validation.MsgExpectedRFC3339Date = "ожидается дата в формате ГГГГ-ММ-ДД"
	validation.MsgExpectedRFC3339Time = "ожидается время в формате ЧЧ:ММ:СС"
	validation.MsgExpectedRFC5322Email = "ожидается адрес почты: %v"
	validation.MsgExpectedRFC5890Hostname = "ожидается имя хоста"
	validation.MsgExpectedRFC2673IPv4 = "ожидается IPv4-адрес"
	validation.MsgExpectedRFC2373IPv6 = "ожидается IPv6-адрес"
	validation.MsgExpectedRFCIPAddr = "ожидается IP-адрес (IPv4 или IPv6)"
	validation.MsgExpectedRFC3986URI = "ожидается ссылка: %v"
	validation.MsgExpectedRFC4122UUID = "ожидается UUID: %v"
	validation.MsgExpectedRFC6570URITemplate = "ожидается шаблон ссылки (RFC 6570)"
	validation.MsgExpectedRFC6901JSONPointer = "ожидается JSON-указатель"
	validation.MsgExpectedRFC6901RelativeJSONPointer = "ожидается относительный JSON-указатель"
	validation.MsgExpectedRegexp = "ожидается регулярное выражение: %v"
	validation.MsgExpectedBePattern = "ожидается строка вида %s"
	validation.MsgExpectedMatchPattern = "строка не подходит под шаблон %s"

	// Ranges and sizes.
	validation.MsgExpectedOneOf = "ожидается одно из значений: \"%s\""
	validation.MsgExpectedMinimumNumber = "число должно быть не меньше %v"
	validation.MsgExpectedExclusiveMinimumNumber = "число должно быть больше %v"
	validation.MsgExpectedMaximumNumber = "число должно быть не больше %v"
	validation.MsgExpectedExclusiveMaximumNumber = "число должно быть меньше %v"
	validation.MsgExpectedNumberBeMultipleOf = "число должно быть кратно %v"
	validation.MsgExpectedMinLength = "не короче %d символов"
	validation.MsgExpectedMaxLength = "не длиннее %d символов"
	validation.MsgExpectedMinItems = "не меньше %d элементов"
	validation.MsgExpectedMaxItems = "не больше %d элементов"
	validation.MsgExpectedMinProperties = "объект должен содержать хотя бы %d полей"
	validation.MsgExpectedMaxProperties = "объект должен содержать не больше %d полей"
	validation.MsgExpectedArrayItemsUnique = "элементы списка не должны повторяться"

	// Composition (oneOf/anyOf/not) — rarely user-facing in this API, but a
	// stray English line would be the odd one out.
	validation.MsgExpectedMatchAtLeastOneSchema = "значение не подходит ни под один из допустимых вариантов"
	validation.MsgExpectedMatchExactlyOneSchema = "значение должно подходить ровно под один из вариантов"
	validation.MsgExpectedNotMatchSchema = "значение не должно подходить под этот вариант"
	validation.MsgExpectedPropertyNameInObject = "такого поля нет в объекте"
}
