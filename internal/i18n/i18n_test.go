package i18n

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"testing"
	"unicode"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
)

// errorBody mirrors huma's RFC 9457 error model as the SPA reads it
// (web/src/api/client.ts prefers detail, then errors[0].message, then title).
type errorBody struct {
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Errors []struct {
		Message  string `json:"message"`
		Location string `json:"location"`
	} `json:"errors"`
}

// testAPI registers the two shapes that produce huma's own English messages:
// a body with a required field, and a typed query parameter.
func testAPI(t *testing.T) humatest.TestAPI {
	t.Helper()
	Install()
	_, api := humatest.New(t, huma.DefaultConfig("test", "1.0.0"))
	huma.Register(api, huma.Operation{
		OperationID: "make", Method: http.MethodPost, Path: "/make",
	}, func(_ context.Context, in *struct {
		Body struct {
			Title   string `json:"title" minLength:"2"`
			Percent int    `json:"percent" minimum:"1"`
		}
	}) (*struct{}, error) {
		return &struct{}{}, nil
	})
	huma.Register(api, huma.Operation{
		OperationID: "list", Method: http.MethodGet, Path: "/list",
	}, func(_ context.Context, in *struct {
		Limit int `query:"limit"`
	}) (*struct{}, error) {
		return &struct{}{}, nil
	})
	return api
}

func decode(t *testing.T, body []byte) errorBody {
	t.Helper()
	var e errorBody
	if err := json.Unmarshal(body, &e); err != nil {
		t.Fatalf("decode error body: %v (%s)", err, body)
	}
	return e
}

// isRussian reports whether s carries Cyrillic text. Messages legitimately
// interpolate Latin identifiers (field names, formats), so «no Latin» would
// be the wrong test; «has Cyrillic» is what catches an untranslated message —
// including one a huma upgrade introduces under a new variable name.
func isRussian(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Cyrillic, r) {
			return true
		}
	}
	return false
}

func TestValidationMessagesAreRussian(t *testing.T) {
	api := testAPI(t)

	resp := api.Post("/make", map[string]any{"percent": 0})
	if resp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", resp.Code)
	}
	e := decode(t, resp.Body.Bytes())
	if e.Title != "Не удалось обработать" {
		t.Errorf("title = %q, want the Russian status title", e.Title)
	}
	if e.Detail != "проверка данных не прошла" {
		t.Errorf("detail = %q, want the Russian «validation failed»", e.Detail)
	}
	if len(e.Errors) == 0 {
		t.Fatal("no per-field errors returned")
	}
	for _, d := range e.Errors {
		if !isRussian(d.Message) {
			t.Errorf("field error %q at %s is not localized", d.Message, d.Location)
		}
	}
}

// The multipart messages are literals inside huma, so they can only be caught
// on the way out — cover both the plain lookup and the two-value mime one.
func TestUploadMessagesAreRussian(t *testing.T) {
	Install()
	for in, want := range map[string]string{
		"Failed to open file": "не удалось открыть файл",
		"File required":       "нужно приложить файл",
		"Invalid mime type: got text/plain, expected image/png,image/jpeg": "неподдерживаемый тип файла: text/plain, допустимы: image/png,image/jpeg",
		"cannot read multipart form: http: request body too large":         "не удалось прочитать загруженный файл: http: request body too large",
	} {
		if got := translateDetail(in); got != want {
			t.Errorf("translateDetail(%q) = %q, want %q", in, got, want)
		}
	}
	// Anything unknown (module text, DB errors) passes through untouched.
	if got := translateDetail("категория уже выбрана"); got != "категория уже выбрана" {
		t.Errorf("module message was rewritten: %q", got)
	}
}

func TestParamParseMessagesAreRussian(t *testing.T) {
	api := testAPI(t)

	resp := api.Get("/list?limit=abc")
	if resp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", resp.Code)
	}
	e := decode(t, resp.Body.Bytes())
	if len(e.Errors) == 0 || e.Errors[0].Message != "ожидается целое число" {
		t.Errorf("errors = %+v, want the Russian «invalid integer»", e.Errors)
	}
}

// Errors the modules raise themselves go through the same constructor, so the
// title has to be Russian even though the message is written at the call site.
func TestModuleErrorsKeepRussianTitle(t *testing.T) {
	Install()

	for _, tc := range []struct {
		err       huma.StatusError
		wantTitle string
	}{
		{huma.Error404NotFound("не найдено"), "Не найдено"},
		{huma.Error409Conflict("все слоты категорий по тарифу заняты"), "Конфликт"},
		{huma.Error503ServiceUnavailable("распознавание сейчас недоступно"), "Сервис недоступен"},
	} {
		model, ok := tc.err.(*huma.ErrorModel)
		if !ok {
			t.Fatalf("error is %T, want *huma.ErrorModel", tc.err)
		}
		if model.Title != tc.wantTitle {
			t.Errorf("title for %d = %q, want %q", model.Status, model.Title, tc.wantTitle)
		}
		if !isRussian(model.Detail) {
			t.Errorf("detail %q is not localized", model.Detail)
		}
	}
}

// A 5xx must not carry the raw error to the client. huma's default puts it in
// Errors[], which for a database failure is a connection string, host, port,
// role name and SQLSTATE — this is the regression that shipped to production
// and was found by reading a live 500 body.
func TestServerErrorsHideTheirCause(t *testing.T) {
	Install()

	secret := errors.New(`failed to connect to \"user=sharespences database=sharespences\": ` +
		`127.0.0.1:5432: FATAL: role \"sharespences\" does not exist (SQLSTATE 28000)`)

	for _, status := range []int{http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable} {
		model, ok := huma.NewError(status, "unexpected error occurred", secret).(*huma.ErrorModel)
		if !ok {
			t.Fatalf("%d: not an *huma.ErrorModel", status)
		}
		if len(model.Errors) != 0 {
			t.Errorf("%d: errors = %+v, want none — the cause must not reach the client", status, model.Errors)
		}
		body, err := json.Marshal(model)
		if err != nil {
			t.Fatal(err)
		}
		// The whole serialized response, not just Errors[]: a future change
		// that folds the cause into Detail would be the same leak.
		for _, leak := range []string{"SQLSTATE", "sharespences", "5432", "FATAL"} {
			if strings.Contains(string(body), leak) {
				t.Errorf("%d: response body leaks %q: %s", status, leak, body)
			}
		}
		if !isRussian(model.Detail) {
			t.Errorf("%d: detail %q is not localized", status, model.Detail)
		}
	}
}

// Stripping must not mean losing: the operator still needs the cause, so the
// same constructor writes it to the log. Without this the fix would trade a
// disclosure bug for an undebuggable one.
func TestServerErrorsAreLogged(t *testing.T) {
	Install()

	var logged bytes.Buffer
	prevOut, prevFlags := log.Writer(), log.Flags()
	log.SetOutput(&logged)
	log.SetFlags(0)
	t.Cleanup(func() { log.SetOutput(prevOut); log.SetFlags(prevFlags) })

	_ = huma.NewError(http.StatusInternalServerError, "unexpected error occurred", errors.New("boom: SQLSTATE 28000"))

	if got := logged.String(); !strings.Contains(got, "boom: SQLSTATE 28000") {
		t.Errorf("log = %q, want the raw cause recorded server-side", got)
	}
}

// 4xx details are the opposite case: «не заполнено обязательное поле email»
// is written for the user and must survive.
func TestClientErrorsKeepTheirDetails(t *testing.T) {
	Install()

	model, ok := huma.NewError(http.StatusUnprocessableEntity, "validation failed",
		&huma.ErrorDetail{Message: "expected required property email to be present"}).(*huma.ErrorModel)
	if !ok {
		t.Fatal("not an *huma.ErrorModel")
	}
	if len(model.Errors) != 1 {
		t.Fatalf("errors = %+v, want the validation detail kept", model.Errors)
	}
}
