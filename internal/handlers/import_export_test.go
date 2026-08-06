package handlers_test

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"strings"
	"testing"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
	"github.com/xuri/excelize/v2"
	"github.com/zerodha/fastglue"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
	"gorm.io/gorm"
)

func contactsExportRole(t *testing.T, db *gorm.DB, orgID uuid.UUID) *models.CustomRole {
	t.Helper()
	return testutil.CreateTestRoleWithKeys(t, db, orgID, "contacts-exporter", []string{"contacts:export", "contacts:import", "contacts:delete"})
}

func newCSVImportRequest(t *testing.T, csvBody []byte, updateOnDuplicate bool, replaceAll ...bool) *fastglue.Request {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("table", "contacts"))
	require.NoError(t, writer.WriteField("update_on_duplicate", fmt.Sprintf("%t", updateOnDuplicate)))
	if len(replaceAll) > 0 && replaceAll[0] {
		require.NoError(t, writer.WriteField("replace_all", "true"))
	}
	filePart, err := writer.CreateFormFile("file", "kisiler.csv")
	require.NoError(t, err)
	_, err = filePart.Write(csvBody)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req := testutil.NewRequest(t)
	req.RequestCtx.Request.Header.SetMethod("POST")
	req.RequestCtx.Request.Header.SetContentType(writer.FormDataContentType())
	req.RequestCtx.Request.SetBody(body.Bytes())
	return req
}

func encodeUTF16LE(value string) []byte {
	codeUnits := utf16.Encode([]rune(value))
	result := make([]byte, 2+len(codeUnits)*2)
	result[0] = 0xff
	result[1] = 0xfe
	for i, codeUnit := range codeUnits {
		result[2+i*2] = byte(codeUnit)
		result[3+i*2] = byte(codeUnit >> 8)
	}
	return result
}

func decodeSpreadsheetExport(value []byte) string {
	if bytes.HasPrefix(value, []byte{'P', 'K', 0x03, 0x04}) {
		workbook, err := excelize.OpenReader(bytes.NewReader(value))
		if err != nil {
			return ""
		}
		defer workbook.Close() //nolint:errcheck
		sheets := workbook.GetSheetList()
		if len(sheets) == 0 {
			return ""
		}
		rows, err := workbook.GetRows(sheets[0])
		if err != nil {
			return ""
		}
		var result strings.Builder
		result.WriteString("sep=;\r\n")
		writer := csv.NewWriter(&result)
		writer.Comma = ';'
		writer.UseCRLF = true
		for _, row := range rows {
			_ = writer.Write(row)
		}
		writer.Flush()
		return result.String()
	}
	if len(value) >= 3 && value[0] == 0xef && value[1] == 0xbb && value[2] == 0xbf {
		return string(value[3:])
	}
	if len(value) >= 2 && value[0] == 0xff && value[1] == 0xfe {
		value = value[2:]
		codeUnits := make([]uint16, len(value)/2)
		for i := range codeUnits {
			codeUnits[i] = uint16(value[i*2]) | uint16(value[i*2+1])<<8
		}
		return string(utf16.Decode(codeUnits))
	}
	return string(value)
}

// --- GetExportConfig ---

func TestApp_GetExportConfig_Contacts(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	role := contactsExportRole(t, app.DB, org.ID)
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithRoleID(&role.ID))

	req := testutil.NewGETRequest(t)
	testutil.SetAuthContext(req, org.ID, user.ID)
	testutil.SetPathParam(req, "table", "contacts")

	require.NoError(t, app.GetExportConfig(req))
	require.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req))

	var resp struct {
		Data struct {
			Table          string              `json:"table"`
			Columns        []map[string]string `json:"columns"`
			DefaultColumns []string            `json:"default_columns"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(testutil.GetResponseBody(req), &resp))
	assert.Equal(t, "contacts", resp.Data.Table)
	assert.NotEmpty(t, resp.Data.Columns, "columns must be returned")
	assert.NotEmpty(t, resp.Data.DefaultColumns)

	// Sanity: column entries have key + label.
	got := make(map[string]string, len(resp.Data.Columns))
	for _, c := range resp.Data.Columns {
		got[c["key"]] = c["label"]
	}
	assert.Equal(t, "Telefon Numarasi", got["phone_number"])
	assert.Contains(t, got, "tags")
}

func TestApp_GetExportConfig_InvalidTable(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	role := contactsExportRole(t, app.DB, org.ID)
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithRoleID(&role.ID))

	req := testutil.NewGETRequest(t)
	testutil.SetAuthContext(req, org.ID, user.ID)
	testutil.SetPathParam(req, "table", "users") // not in exportConfigs

	require.NoError(t, app.GetExportConfig(req))
	testutil.AssertErrorResponse(t, req, fasthttp.StatusBadRequest, "Invalid table")
}

func TestApp_GetExportConfig_PermissionDenied(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	role := testutil.CreateTestRoleExact(t, app.DB, org.ID, "no-export", false, false, nil)
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithRoleID(&role.ID))

	req := testutil.NewGETRequest(t)
	testutil.SetAuthContext(req, org.ID, user.ID)
	testutil.SetPathParam(req, "table", "contacts")

	require.NoError(t, app.GetExportConfig(req))
	assert.Equal(t, fasthttp.StatusForbidden, testutil.GetResponseStatusCode(req))
}

// --- GetImportConfig ---

func TestApp_GetImportConfig_Contacts(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	role := contactsExportRole(t, app.DB, org.ID)
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithRoleID(&role.ID))

	req := testutil.NewGETRequest(t)
	testutil.SetAuthContext(req, org.ID, user.ID)
	testutil.SetPathParam(req, "table", "contacts")

	require.NoError(t, app.GetImportConfig(req))
	require.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req))

	var resp struct {
		Data struct {
			Table           string              `json:"table"`
			RequiredColumns []map[string]string `json:"required_columns"`
			OptionalColumns []map[string]string `json:"optional_columns"`
			UniqueColumn    string              `json:"unique_column"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(testutil.GetResponseBody(req), &resp))
	assert.Equal(t, "contacts", resp.Data.Table)
	assert.Equal(t, "phone_number", resp.Data.UniqueColumn)
	require.Len(t, resp.Data.RequiredColumns, 1)
	assert.Equal(t, "phone_number", resp.Data.RequiredColumns[0]["key"])
	assert.NotEmpty(t, resp.Data.OptionalColumns)
}

func TestApp_GetImportConfig_InvalidTable(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	role := contactsExportRole(t, app.DB, org.ID)
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithRoleID(&role.ID))

	req := testutil.NewGETRequest(t)
	testutil.SetAuthContext(req, org.ID, user.ID)
	testutil.SetPathParam(req, "table", "made-up")

	require.NoError(t, app.GetImportConfig(req))
	testutil.AssertErrorResponse(t, req, fasthttp.StatusBadRequest, "Invalid table")
}

// --- ExportData ---

func TestApp_ExportData_Contacts_OnlyOwnOrg(t *testing.T) {
	app := newTestApp(t)
	orgA := testutil.CreateTestOrganization(t, app.DB)
	orgB := testutil.CreateTestOrganization(t, app.DB)
	role := contactsExportRole(t, app.DB, orgA.ID)
	userA := testutil.CreateTestUser(t, app.DB, orgA.ID, testutil.WithRoleID(&role.ID))

	// Create contacts in BOTH orgs.
	c1 := testutil.CreateTestContactWith(t, app.DB, orgA.ID, testutil.WithPhoneNumber("+11111111"))
	c2 := testutil.CreateTestContactWith(t, app.DB, orgA.ID, testutil.WithPhoneNumber("+22222222"))
	// Other org contact MUST NOT appear in user A's export.
	other := testutil.CreateTestContactWith(t, app.DB, orgB.ID, testutil.WithPhoneNumber("+99999999"))

	body, _ := json.Marshal(map[string]any{
		"table":   "contacts",
		"columns": []string{"phone_number", "profile_name"},
	})
	req := testutil.NewRequest(t)
	req.RequestCtx.Request.Header.SetContentType("application/json")
	req.RequestCtx.Request.Header.SetMethod("POST")
	req.RequestCtx.Request.SetBody(body)
	testutil.SetAuthContext(req, orgA.ID, userA.ID)

	require.NoError(t, app.ExportData(req))
	require.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req))
	assert.True(t, bytes.HasPrefix(testutil.GetResponseBody(req), []byte{'P', 'K', 0x03, 0x04}),
		"contact export must be a real XLSX workbook")
	csv := decodeSpreadsheetExport(testutil.GetResponseBody(req))

	// Excel separator directive + header + 2 rows from orgA, no orgB row.
	lines := strings.Split(strings.TrimRight(csv, "\n"), "\n")
	require.Len(t, lines, 4, "expected separator directive + header + 2 rows")
	assert.Equal(t, "sep=;\r", lines[0])
	assert.Contains(t, lines[1], "Telefon Numarasi")
	combined := strings.Join(lines[2:], "\n")
	assert.Contains(t, combined, c1.PhoneNumber)
	assert.Contains(t, combined, c2.PhoneNumber)
	assert.NotContains(t, combined, other.PhoneNumber, "other org's contact must not leak into export")

	// Content-Type and download header.
	assert.Equal(t, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		string(req.RequestCtx.Response.Header.Peek("Content-Type")))
	assert.Contains(t, string(req.RequestCtx.Response.Header.Peek("Content-Disposition")), "attachment")
	assert.Contains(t, string(req.RequestCtx.Response.Header.Peek("Content-Disposition")), ".xlsx")
}

func TestApp_ExportData_RejectsDisallowedColumn(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	role := contactsExportRole(t, app.DB, org.ID)
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithRoleID(&role.ID))

	body, _ := json.Marshal(map[string]any{
		"table":   "contacts",
		"columns": []string{"phone_number", "password_hash"}, // not in AllowedColumns
	})
	req := testutil.NewRequest(t)
	req.RequestCtx.Request.Header.SetContentType("application/json")
	req.RequestCtx.Request.Header.SetMethod("POST")
	req.RequestCtx.Request.SetBody(body)
	testutil.SetAuthContext(req, org.ID, user.ID)

	require.NoError(t, app.ExportData(req))
	testutil.AssertErrorResponse(t, req, fasthttp.StatusBadRequest, "not allowed for export")
}

func TestApp_ExportData_PermissionDenied(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	// Role with no contacts:export.
	role := testutil.CreateTestRoleWithKeys(t, app.DB, org.ID, "read-only", []string{"contacts:read"})
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithRoleID(&role.ID))

	body, _ := json.Marshal(map[string]any{"table": "contacts"})
	req := testutil.NewRequest(t)
	req.RequestCtx.Request.Header.SetContentType("application/json")
	req.RequestCtx.Request.Header.SetMethod("POST")
	req.RequestCtx.Request.SetBody(body)
	testutil.SetAuthContext(req, org.ID, user.ID)

	require.NoError(t, app.ExportData(req))
	assert.Equal(t, fasthttp.StatusForbidden, testutil.GetResponseStatusCode(req))
}

func TestApp_ExportData_InvalidTable(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	role := contactsExportRole(t, app.DB, org.ID)
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithRoleID(&role.ID))

	body, _ := json.Marshal(map[string]any{"table": "users"})
	req := testutil.NewRequest(t)
	req.RequestCtx.Request.Header.SetContentType("application/json")
	req.RequestCtx.Request.Header.SetMethod("POST")
	req.RequestCtx.Request.SetBody(body)
	testutil.SetAuthContext(req, org.ID, user.ID)

	require.NoError(t, app.ExportData(req))
	testutil.AssertErrorResponse(t, req, fasthttp.StatusBadRequest, "Invalid table")
}

func TestApp_ExportData_InvalidJSONBody(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	role := contactsExportRole(t, app.DB, org.ID)
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithRoleID(&role.ID))

	req := testutil.NewRequest(t)
	req.RequestCtx.Request.Header.SetContentType("application/json")
	req.RequestCtx.Request.Header.SetMethod("POST")
	req.RequestCtx.Request.SetBody([]byte("not json"))
	testutil.SetAuthContext(req, org.ID, user.ID)

	require.NoError(t, app.ExportData(req))
	testutil.AssertErrorResponse(t, req, fasthttp.StatusBadRequest, "Invalid request body")
}

func TestApp_ExportData_DefaultColumnsWhenEmpty(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	role := contactsExportRole(t, app.DB, org.ID)
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithRoleID(&role.ID))
	contact := testutil.CreateTestContactWith(t, app.DB, org.ID, testutil.WithPhoneNumber("+5555"))
	require.NoError(t, app.DB.Model(contact).Updates(map[string]any{
		"customer_id":  "B2B-7788",
		"profile_name": "MÃ¼ÅŸteri",
		"company_name": "ALORTEK ENERJÄ°",
	}).Error)

	// Empty columns array → should fall back to default columns.
	body, _ := json.Marshal(map[string]any{"table": "contacts"})
	req := testutil.NewRequest(t)
	req.RequestCtx.Request.Header.SetContentType("application/json")
	req.RequestCtx.Request.Header.SetMethod("POST")
	req.RequestCtx.Request.SetBody(body)
	testutil.SetAuthContext(req, org.ID, user.ID)

	require.NoError(t, app.ExportData(req))
	require.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req))
	csv := decodeSpreadsheetExport(testutil.GetResponseBody(req))
	// The default snapshot contains stable IDs and all editable contact fields.
	lines := strings.Split(csv, "\n")
	require.GreaterOrEqual(t, len(lines), 2)
	header := lines[1]
	assert.True(t, strings.HasPrefix(header, "ID;B2B Panel ID;Adi;Soyadi;Gorunen Isim;"),
		"the editable snapshot columns must have a predictable order")
	assert.Contains(t, header, "ID")
	assert.Contains(t, header, "Telefon Numarasi")
	assert.Contains(t, header, "Adi")
	assert.Contains(t, header, "Kategoriler")
	assert.Contains(t, csv, contact.ID.String(), "the downloaded template must contain existing contact IDs")
	assert.Contains(t, csv, "B2B-7788", "the external panel ID must be exported in column B")
	assert.Contains(t, csv, "Müşteri", "reversible legacy mojibake must be repaired on export")
	assert.Contains(t, csv, "ALORTEK ENERJİ", "legacy company names must be repaired on export")
	assert.NotContains(t, csv, "MÃ", "repaired exports must not contain common UTF-8 mojibake")
	for _, char := range header {
		assert.LessOrEqual(t, char, rune(127), "snapshot headers must contain ASCII characters only")
	}
}

func TestApp_ExportData_CSVInjectionEscaped(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	role := contactsExportRole(t, app.DB, org.ID)
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithRoleID(&role.ID))

	// Insert a contact whose name starts with '=' — a classic CSV injection vector.
	require.NoError(t, app.DB.Create(&models.Contact{
		BaseModel:      models.BaseModel{ID: uuid.New()},
		OrganizationID: org.ID,
		PhoneNumber:    "+1234",
		ProfileName:    "=cmd|'/c calc'!A1",
	}).Error)

	body, _ := json.Marshal(map[string]any{
		"table":   "contacts",
		"columns": []string{"profile_name"},
	})
	req := testutil.NewRequest(t)
	req.RequestCtx.Request.Header.SetContentType("application/json")
	req.RequestCtx.Request.Header.SetMethod("POST")
	req.RequestCtx.Request.SetBody(body)
	testutil.SetAuthContext(req, org.ID, user.ID)

	require.NoError(t, app.ExportData(req))
	csv := decodeSpreadsheetExport(testutil.GetResponseBody(req))
	// The dangerous cell must be prefixed with a single quote.
	assert.Contains(t, csv, "'=cmd",
		"cells starting with '=' must be prefixed with a single quote to defuse CSV injection")
}

func TestApp_ImportData_UpdatesChangedSurnameByNormalizedPhone(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	role := contactsExportRole(t, app.DB, org.ID)
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithRoleID(&role.ID))

	contact := &models.Contact{
		BaseModel:      models.BaseModel{ID: uuid.New()},
		OrganizationID: org.ID,
		PhoneNumber:    "+905551112233",
		FirstName:      "Ayşe",
		LastName:       "Yılmaz",
		ProfileName:    "Ayşe Yılmaz",
		HasPurchased:   true,
	}
	require.NoError(t, app.DB.Create(contact).Error)

	csvBody := "\ufeffsep=;\r\nAdı;Soyadı;telefon ülke kodu;telefon numarası;Daha önce satın alma yaptı\r\n;Demir;90;05551112233;\r\n"
	req := newCSVImportRequest(t, []byte(csvBody), true)
	testutil.SetAuthContext(req, org.ID, user.ID)

	require.NoError(t, app.ImportData(req))
	require.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req))

	var result struct {
		Updated int `json:"updated"`
		Created int `json:"created"`
		Errors  int `json:"errors"`
	}
	testutil.ParseEnvelopeResponse(t, req, &result)
	assert.Equal(t, 1, result.Updated)
	assert.Zero(t, result.Created)
	assert.Zero(t, result.Errors)

	var updated models.Contact
	require.NoError(t, app.DB.First(&updated, "id = ?", contact.ID).Error)
	assert.Equal(t, "Ayşe", updated.FirstName, "an omitted first name must be preserved")
	assert.Equal(t, "Demir", updated.LastName)
	assert.Equal(t, "Ayşe Demir", updated.ProfileName)
	assert.True(t, updated.HasPurchased, "an empty cell must not overwrite the existing boolean")

	var count int64
	require.NoError(t, app.DB.Model(&models.Contact{}).Where("organization_id = ?", org.ID).Count(&count).Error)
	assert.EqualValues(t, 1, count, "+90 and digit-only phone variants must match the same contact")
}

func TestApp_ImportData_CreatesContactFromTurkishExcelTemplate(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	role := contactsExportRole(t, app.DB, org.ID)
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithRoleID(&role.ID))

	csvBody := "\ufeffsep=;\r\nMüşteri ID;Adı;Soyadı;telefon ülke kodu;telefon numarası;Şehir;İlçe;Daha önce satın alma yaptı;Etiketler\r\nM-1001;Çağla;Şen;90;05552223344;İstanbul;Şişli;Evet;vip,müşteri\r\n"
	req := newCSVImportRequest(t, encodeUTF16LE(csvBody), true)
	testutil.SetAuthContext(req, org.ID, user.ID)

	require.NoError(t, app.ImportData(req))
	require.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req))

	var result struct {
		Created int `json:"created"`
		Errors  int `json:"errors"`
	}
	testutil.ParseEnvelopeResponse(t, req, &result)
	assert.Equal(t, 1, result.Created)
	assert.Zero(t, result.Errors)

	var contact models.Contact
	require.NoError(t, app.DB.Where("organization_id = ?", org.ID).First(&contact).Error)
	assert.Equal(t, "905552223344", contact.PhoneNumber)
	assert.Equal(t, "M-1001", contact.CustomerID)
	assert.Equal(t, "Çağla", contact.FirstName)
	assert.Equal(t, "Şen", contact.LastName)
	assert.Equal(t, "Çağla Şen", contact.ProfileName)
	assert.Equal(t, "İstanbul", contact.City)
	assert.Equal(t, "Şişli", contact.District)
	assert.True(t, contact.HasPurchased)
	assert.Equal(t, models.JSONBArray{"vip", "müşteri"}, contact.Tags)
}

func TestApp_ImportData_ContactSnapshotReplacesListByID(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	otherOrg := testutil.CreateTestOrganization(t, app.DB)
	role := contactsExportRole(t, app.DB, org.ID)
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithRoleID(&role.ID))

	kept := &models.Contact{
		BaseModel:      models.BaseModel{ID: uuid.New()},
		OrganizationID: org.ID,
		PhoneNumber:    "905551112233",
		FirstName:      "Ayse",
		LastName:       "Yilmaz",
		ProfileName:    "Ayse Yilmaz",
		City:           "Ankara",
	}
	removed := testutil.CreateTestContactWith(t, app.DB, org.ID, testutil.WithPhoneNumber("905559998877"))
	other := testutil.CreateTestContactWith(t, app.DB, otherOrg.ID, testutil.WithPhoneNumber("905550000000"))
	require.NoError(t, app.DB.Create(kept).Error)

	csvBody := fmt.Sprintf(
		"sep=;\r\nID;B2B Panel ID;Adi;Soyadi;Telefon Ulke Kodu;Telefon Numarasi;Sehir;Daha Once Satin Alma Yapti;Kategoriler\r\n"+
			"%s;M-1;Ayse;Demir;90;5551112233;Istanbul;Evet;vip\r\n"+
			";M-2;Mehmet;Kaya;90;5552223344;Izmir;Hayir;yeni\r\n",
		kept.ID,
	)
	req := newCSVImportRequest(t, encodeUTF16LE(csvBody), false, true)
	testutil.SetAuthContext(req, org.ID, user.ID)

	require.NoError(t, app.ImportData(req))
	require.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req))

	var result struct {
		Created int   `json:"created"`
		Updated int   `json:"updated"`
		Deleted int64 `json:"deleted"`
		Errors  int   `json:"errors"`
	}
	testutil.ParseEnvelopeResponse(t, req, &result)
	assert.Equal(t, 1, result.Created)
	assert.Equal(t, 1, result.Updated)
	assert.EqualValues(t, 1, result.Deleted)
	assert.Zero(t, result.Errors)

	var updated models.Contact
	require.NoError(t, app.DB.First(&updated, "id = ?", kept.ID).Error)
	assert.Equal(t, "Demir", updated.LastName)
	assert.Equal(t, "Ayse Demir", updated.ProfileName)
	assert.Equal(t, "Istanbul", updated.City)
	assert.True(t, updated.HasPurchased)

	var newContact models.Contact
	require.NoError(t, app.DB.Where("organization_id = ? AND customer_id = ?", org.ID, "M-2").First(&newContact).Error)
	assert.NotEqual(t, uuid.Nil, newContact.ID)
	assert.Equal(t, "905552223344", newContact.PhoneNumber)

	var deleted models.Contact
	require.ErrorIs(t, app.DB.First(&deleted, "id = ?", removed.ID).Error, gorm.ErrRecordNotFound)
	require.NoError(t, app.DB.Unscoped().First(&deleted, "id = ?", removed.ID).Error)
	assert.True(t, deleted.DeletedAt.Valid)

	var untouched models.Contact
	require.NoError(t, app.DB.First(&untouched, "id = ?", other.ID).Error)
}

func TestApp_ImportData_ContactSnapshotAcceptsTurkishWindowsExcelEncoding(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	role := contactsExportRole(t, app.DB, org.ID)
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithRoleID(&role.ID))

	csvBody := "sep=;\r\nID;B2B Panel ID;Adi;Soyadi;Firma Adi;Telefon Numarasi\r\n" +
		";PANEL-42;ÖZGÜR;ŞEN;GÜNEŞ ENERJİ;905551112233\r\n"
	encoded, _, err := transform.Bytes(charmap.Windows1254.NewEncoder(), []byte(csvBody))
	require.NoError(t, err)
	require.False(t, utf8.Valid(encoded), "fixture must exercise the Windows-1254 fallback")

	req := newCSVImportRequest(t, encoded, false, true)
	testutil.SetAuthContext(req, org.ID, user.ID)

	require.NoError(t, app.ImportData(req))
	require.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req))

	var contact models.Contact
	require.NoError(t, app.DB.Where("organization_id = ?", org.ID).First(&contact).Error)
	assert.Equal(t, "PANEL-42", contact.CustomerID)
	assert.Equal(t, "ÖZGÜR", contact.FirstName)
	assert.Equal(t, "ŞEN", contact.LastName)
	assert.Equal(t, "ÖZGÜR ŞEN", contact.ProfileName)
	assert.Equal(t, "GÜNEŞ ENERJİ", contact.CompanyName)
}

func TestApp_ImportData_ContactSnapshotXLSXRoundTripPreservesTurkishText(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	role := contactsExportRole(t, app.DB, org.ID)
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithRoleID(&role.ID))
	contact := &models.Contact{
		BaseModel:      models.BaseModel{ID: uuid.New()},
		OrganizationID: org.ID,
		CustomerID:     "527",
		FirstName:      "ÖZGÜR",
		LastName:       "ŞEN",
		ProfileName:    "ÖZGÜR ŞEN",
		CompanyName:    "ALORTEK ENERJİ ÜRETİM İTH. İHR. ŞTİ.",
		PhoneNumber:    "905551112233",
		City:           "İstanbul",
		District:       "Bağcılar",
	}
	require.NoError(t, app.DB.Create(contact).Error)

	exportBody, _ := json.Marshal(map[string]any{"table": "contacts"})
	exportReq := testutil.NewRequest(t)
	exportReq.RequestCtx.Request.Header.SetContentType("application/json")
	exportReq.RequestCtx.Request.Header.SetMethod("POST")
	exportReq.RequestCtx.Request.SetBody(exportBody)
	testutil.SetAuthContext(exportReq, org.ID, user.ID)
	require.NoError(t, app.ExportData(exportReq))
	workbookBytes := testutil.GetResponseBody(exportReq)
	require.True(t, bytes.HasPrefix(workbookBytes, []byte{'P', 'K', 0x03, 0x04}))

	importReq := newCSVImportRequest(t, workbookBytes, false, true)
	testutil.SetAuthContext(importReq, org.ID, user.ID)
	require.NoError(t, app.ImportData(importReq))
	require.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(importReq))

	var result struct {
		Updated int `json:"updated"`
		Errors  int `json:"errors"`
	}
	testutil.ParseEnvelopeResponse(t, importReq, &result)
	assert.Equal(t, 1, result.Updated)
	assert.Zero(t, result.Errors)

	var roundTripped models.Contact
	require.NoError(t, app.DB.First(&roundTripped, "id = ?", contact.ID).Error)
	assert.Equal(t, "527", roundTripped.CustomerID)
	assert.Equal(t, "ÖZGÜR", roundTripped.FirstName)
	assert.Equal(t, "ŞEN", roundTripped.LastName)
	assert.Equal(t, "ALORTEK ENERJİ ÜRETİM İTH. İHR. ŞTİ.", roundTripped.CompanyName)
	assert.Equal(t, "İstanbul", roundTripped.City)
	assert.Equal(t, "Bağcılar", roundTripped.District)
	assert.Equal(t, "905551112233", roundTripped.PhoneNumber)
}

func TestApp_ImportData_ContactSnapshotPreservesExplicitDisplayName(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	role := contactsExportRole(t, app.DB, org.ID)
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithRoleID(&role.ID))

	contact := &models.Contact{
		BaseModel:      models.BaseModel{ID: uuid.New()},
		OrganizationID: org.ID,
		PhoneNumber:    "905551112233",
		ProfileName:    "WhatsApp Gorunen Ad",
	}
	require.NoError(t, app.DB.Create(contact).Error)

	csvBody := fmt.Sprintf(
		"sep=;\r\nID;Adi;Soyadi;Gorunen Isim;Telefon Numarasi\r\n"+
			"%s;;;WhatsApp Gorunen Ad;905551112233\r\n",
		contact.ID,
	)
	req := newCSVImportRequest(t, encodeUTF16LE(csvBody), false, true)
	testutil.SetAuthContext(req, org.ID, user.ID)

	require.NoError(t, app.ImportData(req))
	require.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req))

	var updated models.Contact
	require.NoError(t, app.DB.First(&updated, "id = ?", contact.ID).Error)
	assert.Equal(t, "WhatsApp Gorunen Ad", updated.ProfileName)
}

func TestApp_ImportData_ContactSnapshotRollsBackOnAnyError(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	role := contactsExportRole(t, app.DB, org.ID)
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithRoleID(&role.ID))

	kept := &models.Contact{
		BaseModel:      models.BaseModel{ID: uuid.New()},
		OrganizationID: org.ID,
		PhoneNumber:    "905551112233",
		FirstName:      "Ayse",
		LastName:       "Yilmaz",
		ProfileName:    "Ayse Yilmaz",
	}
	shouldRemain := testutil.CreateTestContactWith(t, app.DB, org.ID, testutil.WithPhoneNumber("905559998877"))
	require.NoError(t, app.DB.Create(kept).Error)

	// The second row has an empty ID but reuses an existing phone number.
	csvBody := fmt.Sprintf(
		"sep=;\r\nID;Adi;Soyadi;Telefon Ulke Kodu;Telefon Numarasi\r\n"+
			"%s;Ayse;Degisti;90;5551112233\r\n"+
			";Yeni;Kisi;90;5551112233\r\n",
		kept.ID,
	)
	req := newCSVImportRequest(t, encodeUTF16LE(csvBody), false, true)
	testutil.SetAuthContext(req, org.ID, user.ID)

	require.NoError(t, app.ImportData(req))
	require.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req))

	var result struct {
		Created int `json:"created"`
		Updated int `json:"updated"`
		Deleted int `json:"deleted"`
		Errors  int `json:"errors"`
	}
	testutil.ParseEnvelopeResponse(t, req, &result)
	assert.Zero(t, result.Created)
	assert.Zero(t, result.Updated)
	assert.Zero(t, result.Deleted)
	assert.Positive(t, result.Errors)

	var unchanged models.Contact
	require.NoError(t, app.DB.First(&unchanged, "id = ?", kept.ID).Error)
	assert.Equal(t, "Yilmaz", unchanged.LastName)
	require.NoError(t, app.DB.First(&unchanged, "id = ?", shouldRemain.ID).Error)
}

func TestApp_ImportData_ContactSnapshotRequiresIDColumn(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	role := contactsExportRole(t, app.DB, org.ID)
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithRoleID(&role.ID))

	csvBody := "sep=;\r\nAdi;Soyadi;Telefon Numarasi\r\nAyse;Yilmaz;905551112233\r\n"
	req := newCSVImportRequest(t, encodeUTF16LE(csvBody), false, true)
	testutil.SetAuthContext(req, org.ID, user.ID)

	require.NoError(t, app.ImportData(req))
	testutil.AssertErrorResponse(t, req, fasthttp.StatusBadRequest, "ID column is required")
}

func TestApp_ImportData_ContactSnapshotDoesNotDeleteOnHeaderOnlyFile(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	role := contactsExportRole(t, app.DB, org.ID)
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithRoleID(&role.ID))
	existing := testutil.CreateTestContactWith(t, app.DB, org.ID, testutil.WithPhoneNumber("905551112233"))

	csvBody := "sep=;\r\nID;Adi;Soyadi;Telefon Numarasi\r\n"
	req := newCSVImportRequest(t, encodeUTF16LE(csvBody), false, true)
	testutil.SetAuthContext(req, org.ID, user.ID)

	require.NoError(t, app.ImportData(req))
	testutil.AssertErrorResponse(t, req, fasthttp.StatusBadRequest, "at least one data row")

	var unchanged models.Contact
	require.NoError(t, app.DB.First(&unchanged, "id = ?", existing.ID).Error)
}

func TestApp_ImportData_ContactSnapshotRequiresDeletePermission(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	role := testutil.CreateTestRoleWithKeys(t, app.DB, org.ID, "contacts-import-only", []string{"contacts:import"})
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithRoleID(&role.ID))

	csvBody := "sep=;\r\nID;Adi;Soyadi;Telefon Numarasi\r\n;Ayse;Yilmaz;905551112233\r\n"
	req := newCSVImportRequest(t, encodeUTF16LE(csvBody), false, true)
	testutil.SetAuthContext(req, org.ID, user.ID)

	require.NoError(t, app.ImportData(req))
	testutil.AssertErrorResponse(t, req, fasthttp.StatusForbidden, "replace the contact list")
}
