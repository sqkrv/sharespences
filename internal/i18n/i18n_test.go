package i18n

import (
	"context"
	"encoding/json"
	"net/http"
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
