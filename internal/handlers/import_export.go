package handlers

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/internal/utils"
	"github.com/valyala/fasthttp"
	"github.com/xuri/excelize/v2"
	"github.com/zerodha/fastglue"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
	"gorm.io/gorm"
)

// ExportConfig defines allowed tables and their exportable columns
type ExportConfig struct {
	Model           any
	Resource        string // For permission check
	AllowedColumns  []string
	DefaultColumns  []string
	ColumnLabels    map[string]string // Column name -> CSV header label
	ColumnTransform map[string]func(any) string
}

// ImportConfig defines allowed tables and their importable columns
type ImportConfig struct {
	Model           any
	Resource        string // For permission check
	RequiredColumns []string
	OptionalColumns []string
	ColumnTransform map[string]func(string) (any, error)
	UniqueColumn    string // Column to check for duplicates (e.g., "phone_number")
	BeforeCreate    func(db *gorm.DB, orgID uuid.UUID, record map[string]any) error
	ColumnAliases   map[string][]string
}

// Supported export/import configurations
var exportConfigs = map[string]ExportConfig{
	"contacts": {
		Model:    &models.Contact{},
		Resource: "contacts",
		AllowedColumns: []string{
			"id", "customer_id", "first_name", "last_name", "profile_name", "company_name", "email", "phone_country_code", "phone_number",
			"tax_office", "tax_number", "postal_code", "city", "district", "address", "purchase_score", "has_purchased", "note", "whats_app_account", "tags",
			"assigned_user_id", "last_message_at", "created_at", "updated_at",
		},
		DefaultColumns: []string{"id", "customer_id", "first_name", "last_name", "profile_name", "company_name", "email", "phone_country_code", "phone_number", "tax_office", "tax_number", "postal_code", "city", "district", "address", "purchase_score", "has_purchased", "tags", "note"},
		ColumnLabels: map[string]string{
			"id":                 "ID",
			"customer_id":        "B2B Panel ID",
			"first_name":         "Adi",
			"last_name":          "Soyadi",
			"company_name":       "Firma Adi",
			"email":              "E-posta",
			"phone_country_code": "Telefon Ulke Kodu",
			"phone_number":       "Telefon Numarasi",
			"tax_office":         "Vergi Dairesi",
			"tax_number":         "Vergi veya TC Kimlik No",
			"postal_code":        "Posta Kodu",
			"city":               "Sehir",
			"district":           "Ilce",
			"address":            "Acik Adres",
			"purchase_score":     "Satin Alma Puani 0-100",
			"has_purchased":      "Daha Once Satin Alma Yapti",
			"tags":               "Kategoriler",
			"note":               "Not",
			"profile_name":       "Gorunen Isim",
			"whats_app_account":  "WhatsApp Hesabi",
			"assigned_user_id":   "Atanan Kullanici ID",
			"last_message_at":    "Son Mesaj Zamani",
			"created_at":         "Olusturulma Zamani",
			"updated_at":         "Guncellenme Zamani",
		},
		ColumnTransform: map[string]func(any) string{
			"tags": func(v any) string {
				if v == nil {
					return ""
				}
				if tags, ok := v.(models.JSONBArray); ok {
					var tagStrs []string
					for _, t := range tags {
						if s, ok := t.(string); ok {
							tagStrs = append(tagStrs, s)
						}
					}
					return strings.Join(tagStrs, ",")
				}
				var encoded []byte
				switch value := v.(type) {
				case []byte:
					encoded = value
				case string:
					encoded = []byte(value)
				}
				if len(encoded) > 0 {
					var tags []string
					if json.Unmarshal(encoded, &tags) == nil {
						return strings.Join(tags, ",")
					}
				}
				return ""
			},
			"last_message_at": func(v any) string {
				if v == nil {
					return ""
				}
				if t, ok := v.(*time.Time); ok && t != nil {
					return t.Format(time.RFC3339)
				}
				return ""
			},
			"created_at": func(v any) string {
				if t, ok := v.(time.Time); ok {
					return t.Format(time.RFC3339)
				}
				return ""
			},
			"updated_at": func(v any) string {
				if t, ok := v.(time.Time); ok {
					return t.Format(time.RFC3339)
				}
				return ""
			},
			"assigned_user_id": func(v any) string {
				if v == nil {
					return ""
				}
				if id, ok := v.(*uuid.UUID); ok && id != nil {
					return id.String()
				}
				return ""
			},
			"has_purchased": func(v any) string {
				if purchased, ok := v.(bool); ok && purchased {
					return "Evet"
				}
				return "Hayir"
			},
		},
	},
	"tags": {
		Model:          &models.Tag{},
		Resource:       "tags",
		AllowedColumns: []string{"name", "color", "description", "created_at"},
		DefaultColumns: []string{"name", "color", "description"},
		ColumnLabels: map[string]string{
			"name":        "Name",
			"color":       "Color",
			"description": "Description",
			"created_at":  "Created At",
		},
	},
}

var importConfigs = map[string]ImportConfig{
	"contacts": {
		Model:           &models.Contact{},
		Resource:        "contacts",
		RequiredColumns: []string{"phone_number"},
		OptionalColumns: []string{"id", "phone_country_code", "customer_id", "first_name", "last_name", "profile_name", "company_name", "email", "tax_office", "tax_number", "postal_code", "city", "district", "address", "purchase_score", "has_purchased", "tags", "note", "whats_app_account", "assigned_user_id"},
		UniqueColumn:    "phone_number",
		ColumnAliases: map[string][]string{
			"id": {"ID"}, "customer_id": {"Müşteri ID", "Musteri ID"}, "first_name": {"Adı", "Adi"}, "last_name": {"Soyadı", "Soyadi"},
			"profile_name": {"İsim", "Isim", "Görünen İsim", "Gorunen Isim"}, "company_name": {"Firma Adı", "Firma Adi", "Firma / Ünvan"}, "email": {"e-posta", "E-posta"},
			"phone_country_code": {"telefon ülke kodu", "Telefon Ülke Kodu", "Telefon Ulke Kodu"}, "phone_number": {"telefon numarası", "Telefon Numarası", "Telefon Numarasi"},
			"tax_office": {"Vergi dairesi"}, "tax_number": {"Vergi / T.C. kimlik no", "Vergi veya TC Kimlik No"}, "postal_code": {"Posta kodu"},
			"city": {"Şehir", "Sehir"}, "district": {"İlçe", "Ilce"}, "address": {"Açık adres", "Acik adres"},
			"purchase_score": {"Satın alma puanı (0-100)", "Satin Alma Puani 0-100"}, "has_purchased": {"Daha önce satın alma yaptı", "Daha Once Satin Alma Yapti"},
			"tags": {"Kategoriler", "Etiketler"}, "note": {"Not"}, "whats_app_account": {"WhatsApp Hesabı"},
			"assigned_user_id": {"Atanan Kullanıcı ID"},
		},
		ColumnTransform: map[string]func(string) (any, error){
			"id": func(s string) (any, error) {
				s = strings.TrimSpace(s)
				if s == "" {
					return nil, nil
				}
				parsed, err := uuid.Parse(s)
				if err != nil {
					return nil, fmt.Errorf("invalid ID: %s", s)
				}
				return parsed, nil
			},
			"phone_number": func(s string) (any, error) {
				phone := digitsOnly(s)
				if phone == "" {
					return nil, fmt.Errorf("phone number is required")
				}
				return phone, nil
			},
			"phone_country_code": func(s string) (any, error) {
				code := digitsOnly(s)
				if code == "" {
					return nil, nil
				}
				return code, nil
			},
			"purchase_score": func(s string) (any, error) {
				if strings.TrimSpace(s) == "" {
					return nil, nil
				}
				var score int
				if _, err := fmt.Sscanf(strings.TrimSpace(s), "%d", &score); err != nil || score < 0 || score > 100 {
					return nil, fmt.Errorf("must be between 0 and 100")
				}
				return score, nil
			},
			"has_purchased": func(s string) (any, error) {
				switch strings.ToLower(strings.TrimSpace(s)) {
				case "1", "true", "evet", "yes":
					return true, nil
				case "":
					// Empty cells must not erase the value of an existing contact.
					// New contacts already receive the model's false default.
					return nil, nil
				case "0", "false", "hayır", "hayir", "no":
					return false, nil
				}
				return nil, fmt.Errorf("must be Evet/Hayır or true/false")
			},
			"assigned_user_id": func(s string) (any, error) {
				s = strings.TrimSpace(s)
				if s == "" {
					return nil, nil
				}
				parsed, err := uuid.Parse(s)
				if err != nil {
					return nil, fmt.Errorf("invalid user ID: %s", s)
				}
				return &parsed, nil
			},
			"tags": func(s string) (any, error) {
				if s == "" {
					return nil, nil
				}
				parts := strings.Split(s, ",")
				tags := make(models.JSONBArray, 0, len(parts))
				for _, p := range parts {
					p = strings.TrimSpace(p)
					if p != "" {
						tags = append(tags, p)
					}
				}
				return tags, nil
			},
		},
	},
	"tags": {
		Model:           &models.Tag{},
		Resource:        "tags",
		RequiredColumns: []string{"name"},
		OptionalColumns: []string{"color", "description"},
		UniqueColumn:    "name",
	},
}

// ExportRequest represents an export request
type ExportRequest struct {
	Table   string            `json:"table"`
	Columns []string          `json:"columns"`
	Filters map[string]string `json:"filters"`
	Format  string            `json:"format"` // csv (default), json
}

// ExportData handles generic data export
func (a *App) ExportData(r *fastglue.Request) error {
	orgID, userID, err := a.getOrgAndUserID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}

	var req ExportRequest
	if err := json.Unmarshal(r.RequestCtx.PostBody(), &req); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Invalid request body", nil, "")
	}

	// Get export config
	config, ok := exportConfigs[req.Table]
	if !ok {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Invalid table", nil, "")
	}

	// Check permission
	if !a.HasPermission(userID, config.Resource, models.ActionExport, orgID) {
		return r.SendErrorEnvelope(fasthttp.StatusForbidden, "You do not have permission to export "+req.Table, nil, "")
	}

	// Validate and set columns
	columns := req.Columns
	if len(columns) == 0 {
		columns = config.DefaultColumns
	}

	// Validate columns against allowed set
	allowedSet := make(map[string]bool)
	for _, col := range config.AllowedColumns {
		allowedSet[col] = true
	}
	requestedCols := make(map[string]bool, len(columns))
	for _, col := range columns {
		if !allowedSet[col] {
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, fmt.Sprintf("Column '%s' is not allowed for export", col), nil, "")
		}
		requestedCols[col] = true
	}

	// Build query
	query := a.DB.Model(config.Model).Where("organization_id = ?", orgID)

	// Apply filters
	if search, ok := req.Filters["search"]; ok && search != "" {
		searchPattern := "%" + search + "%"
		switch req.Table {
		case "contacts":
			// Use ILIKE for case-insensitive search on profile_name
			query = query.Where("phone_number LIKE ? OR profile_name ILIKE ?", searchPattern, searchPattern)
		case "tags":
			query = query.Where("name ILIKE ? OR description ILIKE ?", searchPattern, searchPattern)
		}
	}

	if tags, ok := req.Filters["tags"]; ok && tags != "" {
		tagList := strings.Split(tags, ",")
		conditions := make([]string, 0, len(tagList))
		args := make([]any, 0, len(tagList))
		for _, tag := range tagList {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				// Use proper JSONB containment with explicit cast
				conditions = append(conditions, "tags @> ?::jsonb")
				tagJSON, _ := json.Marshal([]string{tag})
				args = append(args, string(tagJSON))
			}
		}
		if len(conditions) > 0 {
			query = query.Where("("+strings.Join(conditions, " OR ")+")", args...)
		}
	}

	// Select only needed columns.
	// Build the list from server-controlled AllowedColumns to prevent SQL injection.
	// This ensures only server-defined strings are passed to GORM, not user input.
	safeColumns := make([]string, 0, len(columns))
	for _, col := range config.AllowedColumns {
		if requestedCols[col] {
			safeColumns = append(safeColumns, col)
		}
	}
	selectCols := append([]string(nil), safeColumns...)
	query = query.Select(selectCols)

	// Execute query
	rows, err := query.Rows()
	if err != nil {
		a.Log.Error("Failed to export data", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to export data", nil, "")
	}
	defer rows.Close() //nolint:errcheck

	// Write header using safe (server-controlled) column names
	header := make([]string, len(safeColumns))
	for i, col := range safeColumns {
		if label, ok := config.ColumnLabels[col]; ok {
			header[i] = label
		} else {
			header[i] = col
		}
	}
	exportRows := make([][]string, 0, 256)
	exportRows = append(exportRows, header)

	// Write rows
	for rows.Next() {
		// Create a slice of any to scan into
		values := make([]any, len(selectCols))
		valuePtrs := make([]any, len(selectCols))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			continue
		}

		// Convert to CSV row.
		csvRow := make([]string, len(safeColumns))
		for i, col := range safeColumns {
			val := values[i]

			// Apply transform if available
			if transform, ok := config.ColumnTransform[col]; ok {
				csvRow[i] = transform(val)
			} else {
				csvRow[i] = formatExportValue(val)
			}
			// Repair reversible legacy mojibake while exporting. Re-importing
			// this snapshot then also cleans the stored contact data.
			csvRow[i] = repairExcelMojibake(csvRow[i])
		}
		// Apply phone masking for contacts export
		if req.Table == "contacts" && a.ShouldMaskPhoneNumbers(orgID) {
			for i, col := range safeColumns {
				switch col {
				case "phone_number":
					csvRow[i] = utils.MaskPhoneNumber(csvRow[i])
				case "profile_name":
					csvRow[i] = utils.MaskIfPhoneNumber(csvRow[i])
				}
			}
		}

		// Escape CSV injection: prefix dangerous first chars with a single quote
		// Only escape '=' and '@' which trigger formulas. '+' and '-' are skipped
		// because they appear in legitimate data (phone numbers, negative values).
		for j, cell := range csvRow {
			if len(cell) > 0 && (cell[0] == '=' || cell[0] == '@') {
				csvRow[j] = "'" + cell
			}
		}
		exportRows = append(exportRows, csvRow)
	}

	if req.Table == "contacts" {
		return sendContactWorkbook(r, safeColumns, exportRows)
	}

	// Non-contact exports remain CSV.
	var buf strings.Builder
	buf.WriteString("sep=;\r\n")
	writer := csv.NewWriter(&buf)
	writer.Comma = ';'
	writer.UseCRLF = true
	for _, row := range exportRows {
		_ = writer.Write(row)
	}
	writer.Flush()

	// Set response headers for CSV download
	filename := fmt.Sprintf("%s_export_%s.csv", req.Table, time.Now().Format("20060102_150405"))
	r.RequestCtx.Response.Header.Set("Content-Type", "text/csv; charset=utf-8")
	r.RequestCtx.Response.Header.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	r.RequestCtx.SetBody(append([]byte{0xef, 0xbb, 0xbf}, []byte(buf.String())...))

	return nil
}

func sendContactWorkbook(r *fastglue.Request, columns []string, rows [][]string) error {
	workbook := excelize.NewFile()
	defer workbook.Close() //nolint:errcheck

	const sheet = "Kisiler"
	workbook.SetSheetName(workbook.GetSheetName(0), sheet)

	headerStyle, err := workbook.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"1F4E78"}},
		Alignment: &excelize.Alignment{Vertical: "center"},
	})
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to create Excel template", nil, "")
	}
	textStyle, err := workbook.NewStyle(&excelize.Style{NumFmt: 49})
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to create Excel template", nil, "")
	}

	for rowIndex, row := range rows {
		for columnIndex, value := range row {
			cell, cellErr := excelize.CoordinatesToCellName(columnIndex+1, rowIndex+1)
			if cellErr != nil {
				return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to create Excel template", nil, "")
			}
			if rowIndex > 0 && columns[columnIndex] == "customer_id" &&
				value != "" && digitsOnly(value) == value && !strings.HasPrefix(value, "0") {
				if numericID, parseErr := strconv.ParseInt(value, 10, 64); parseErr == nil {
					_ = workbook.SetCellInt(sheet, cell, numericID)
					continue
				}
			}
			_ = workbook.SetCellStr(sheet, cell, value)
		}
	}

	lastColumn, _ := excelize.ColumnNumberToName(len(columns))
	lastRow := len(rows)
	if lastRow < 1 {
		lastRow = 1
	}
	_ = workbook.SetCellStyle(sheet, "A1", lastColumn+"1", headerStyle)
	if lastRow > 1 {
		_ = workbook.SetCellStyle(sheet, "A2", lastColumn+strconv.Itoa(lastRow), textStyle)
	}
	_ = workbook.SetRowHeight(sheet, 1, 24)
	_ = workbook.AutoFilter(sheet, "A1:"+lastColumn+strconv.Itoa(lastRow), []excelize.AutoFilterOptions{})
	_ = workbook.SetPanes(sheet, &excelize.Panes{
		Freeze:      true,
		XSplit:      2,
		YSplit:      1,
		TopLeftCell: "C2",
		ActivePane:  "bottomRight",
	})

	for index, column := range columns {
		columnName, _ := excelize.ColumnNumberToName(index + 1)
		width := 18.0
		switch column {
		case "id":
			width = 38
		case "customer_id":
			width = 18
		case "email", "company_name", "profile_name":
			width = 28
		case "address", "note":
			width = 45
		case "phone_country_code":
			width = 20
		}
		_ = workbook.SetColWidth(sheet, columnName, columnName, width)
	}

	buffer, err := workbook.WriteToBuffer()
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to create Excel template", nil, "")
	}
	filename := fmt.Sprintf("kisiler_tam_liste_%s.xlsx", time.Now().Format("20060102_150405"))
	r.RequestCtx.Response.Header.Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	r.RequestCtx.Response.Header.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	r.RequestCtx.SetBody(buffer.Bytes())
	return nil
}

// ImportDataRequest represents an import request metadata
type ImportDataRequest struct {
	Table         string            `json:"table"`
	ColumnMapping map[string]string `json:"column_mapping"` // CSV header -> DB column
	UpdateOnDup   bool              `json:"update_on_duplicate"`
}

// ImportData handles generic data import
func (a *App) ImportData(r *fastglue.Request) error {
	orgID, userID, err := a.getOrgAndUserID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}

	// Parse multipart form
	form, err := r.RequestCtx.MultipartForm()
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Invalid multipart form", nil, "")
	}

	// Get table name
	tableValues := form.Value["table"]
	if len(tableValues) == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "table is required", nil, "")
	}
	tableName := tableValues[0]

	// Get import config
	config, ok := importConfigs[tableName]
	if !ok {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Invalid table", nil, "")
	}

	// Check permission
	if !a.HasPermission(userID, config.Resource, models.ActionImport, orgID) {
		return r.SendErrorEnvelope(fasthttp.StatusForbidden, "You do not have permission to import "+tableName, nil, "")
	}

	// Get update_on_duplicate flag
	updateOnDup := false
	if updateValues := form.Value["update_on_duplicate"]; len(updateValues) > 0 {
		updateOnDup = updateValues[0] == "true"
	}
	replaceAll := false
	if replaceValues := form.Value["replace_all"]; len(replaceValues) > 0 {
		replaceAll = replaceValues[0] == "true"
	}
	if replaceAll {
		if tableName != "contacts" {
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "replace_all is only supported for contacts", nil, "")
		}
		if !a.HasPermission(userID, config.Resource, models.ActionDelete, orgID) {
			return r.SendErrorEnvelope(fasthttp.StatusForbidden, "You do not have permission to replace the contact list", nil, "")
		}
	}

	// Get column mapping (optional)
	columnMapping := make(map[string]string)
	if mappingValues := form.Value["column_mapping"]; len(mappingValues) > 0 {
		_ = json.Unmarshal([]byte(mappingValues[0]), &columnMapping)
	}

	// Get CSV file
	files := form.File["file"]
	if len(files) == 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "file is required", nil, "")
	}
	fileHeader := files[0]

	file, err := fileHeader.Open()
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Failed to read file", nil, "")
	}
	defer file.Close() //nolint:errcheck

	// Limit CSV file size to 10MB
	const maxCSVSize = 10 << 20
	fileBytes, err := io.ReadAll(io.LimitReader(file, maxCSVSize+1))
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Failed to read file", nil, "")
	}
	if len(fileBytes) > maxCSVSize {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Spreadsheet file exceeds 10 MB", nil, "")
	}
	fileBytes, err = normalizeSpreadsheetUpload(fileBytes, fileHeader.Filename)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, err.Error(), nil, "")
	}
	fileBytes, err = decodeSpreadsheetCSV(fileBytes)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Invalid CSV character encoding", nil, "")
	}

	// Parse CSV
	reader := csv.NewReader(bytes.NewReader(fileBytes))
	// Spreadsheet applications may omit trailing empty optional cells. Accept
	// variable-width records and map whichever columns are present.
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true

	// Read header
	header, err := reader.Read()
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Failed to read CSV header", nil, "")
	}
	// Excel writes a sep=; directive so the file opens in separate columns even
	// when the operating system's list separator differs. Honour that directive.
	if len(header) == 1 && strings.EqualFold(strings.TrimSpace(strings.TrimPrefix(header[0], "\ufeff")), "sep=;") {
		reader.Comma = ';'
		header, err = reader.Read()
		if err != nil {
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Failed to read CSV header", nil, "")
		}
	} else if len(header) == 1 && strings.Contains(header[0], ";") {
		// Also accept semicolon CSV files that do not contain Excel's directive.
		reader.Comma = ';'
		header = strings.Split(header[0], ";")
	}

	// Build column index mapping
	colIndex := make(map[string]int)
	for i, h := range header {
		h = repairExcelMojibake(strings.TrimSpace(strings.TrimPrefix(h, "\ufeff")))
		// Apply column mapping if provided
		if mapped, ok := columnMapping[h]; ok {
			colIndex[mapped] = i
		} else {
			// Try to match by lowercase
			colIndex[strings.ToLower(h)] = i
		}
	}

	// Validate required columns exist
	for _, reqCol := range config.RequiredColumns {
		found := false
		for col := range colIndex {
			if strings.EqualFold(col, reqCol) || strings.EqualFold(col, strings.ReplaceAll(reqCol, "_", " ")) ||
				strings.EqualFold(col, config.getColumnLabel(reqCol)) || config.matchesAlias(reqCol, col) {
				found = true
				break
			}
		}
		if !found {
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, fmt.Sprintf("Required column '%s' not found in CSV", reqCol), nil, "")
		}
	}

	// Normalize column index keys
	normalizedIndex := make(map[string]int)
	for col, idx := range colIndex {
		// Match against allowed columns
		for _, allowed := range append(config.RequiredColumns, config.OptionalColumns...) {
			if strings.EqualFold(col, allowed) ||
				strings.EqualFold(col, strings.ReplaceAll(allowed, "_", " ")) ||
				strings.EqualFold(col, config.getColumnLabel(allowed)) || config.matchesAlias(allowed, col) {
				normalizedIndex[allowed] = idx
				break
			}
		}
	}

	if replaceAll {
		return a.importContactSnapshot(r, reader, config, normalizedIndex, orgID)
	}

	// Process rows (limit to 10,000)
	const maxImportRows = 10000
	var created, updated, skipped, errors int
	var errorMessages []string

	rowNum := 1
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		rowNum++
		if rowNum > maxImportRows+1 { // +1 for header row
			errorMessages = append(errorMessages, fmt.Sprintf("Import limited to %d rows", maxImportRows))
			break
		}
		if err != nil {
			errors++
			errorMessages = append(errorMessages, fmt.Sprintf("Row %d: failed to parse", rowNum))
			continue
		}

		// Build record map
		recordMap := make(map[string]any)
		recordMap["organization_id"] = orgID

		hasError := false
		for col, idx := range normalizedIndex {
			if idx >= len(record) {
				continue
			}
			val := repairExcelMojibake(strings.TrimSpace(record[idx]))

			// Validate field length
			if len(val) > 10000 {
				hasError = true
				errorMessages = append(errorMessages, fmt.Sprintf("Row %d: %s exceeds max length", rowNum, col))
				break
			}

			// Apply transform if available
			if transform, ok := config.ColumnTransform[col]; ok {
				transformed, err := transform(val)
				if err != nil {
					hasError = true
					errorMessages = append(errorMessages, fmt.Sprintf("Row %d: %s - %s", rowNum, col, err.Error()))
					break
				}
				if transformed != nil {
					recordMap[col] = transformed
				}
			} else if val != "" {
				recordMap[col] = val
			}
		}

		if hasError {
			errors++
			continue
		}

		if tableName == "contacts" {
			country, _ := recordMap["phone_country_code"].(string)
			phone, _ := recordMap["phone_number"].(string)
			normalizedPhone, normalizeErr := normalizeImportedPhone(country, phone)
			if normalizeErr != nil {
				errors++
				errorMessages = append(errorMessages, fmt.Sprintf("Row %d: phone_number - %s", rowNum, normalizeErr.Error()))
				continue
			}
			recordMap["phone_number"] = normalizedPhone
		}

		// Check for required fields
		for _, reqCol := range config.RequiredColumns {
			if _, ok := recordMap[reqCol]; !ok {
				hasError = true
				errorMessages = append(errorMessages, fmt.Sprintf("Row %d: missing required field '%s'", rowNum, reqCol))
				break
			}
		}

		if hasError {
			errors++
			continue
		}

		// Check for duplicate based on unique column
		if config.UniqueColumn != "" {
			uniqueVal := recordMap[config.UniqueColumn]
			var existing any

			// Use reflection to create a new instance of the model type
			modelType := reflect.TypeOf(config.Model).Elem()
			existing = reflect.New(modelType).Interface()

			findQuery := a.DB.Where("organization_id = ?", orgID)
			if tableName == "contacts" && config.UniqueColumn == "phone_number" {
				phone := fmt.Sprint(uniqueVal)
				findQuery = findQuery.Where("phone_number IN ?", []string{phone, "+" + phone})
			} else {
				findQuery = findQuery.Where(config.UniqueColumn+" = ?", uniqueVal)
			}

			err := findQuery.First(existing).Error
			if err == nil {
				// Record exists
				if updateOnDup {
					if tableName == "contacts" {
						applyImportedContactDisplayName(recordMap, existing.(*models.Contact))
					}
					// Update existing record
					delete(recordMap, "organization_id")
					delete(recordMap, config.UniqueColumn)
					if len(recordMap) > 0 {
						if err := a.DB.Model(existing).Updates(recordMap).Error; err != nil {
							errors++
							errorMessages = append(errorMessages, fmt.Sprintf("Row %d: failed to update", rowNum))
						} else {
							updated++
						}
					} else {
						skipped++
					}
				} else {
					skipped++
				}
				continue
			}
		}

		if tableName == "contacts" {
			applyImportedContactDisplayName(recordMap, nil)
		}

		// Run BeforeCreate hook if defined
		if config.BeforeCreate != nil {
			if err := config.BeforeCreate(a.DB, orgID, recordMap); err != nil {
				errors++
				errorMessages = append(errorMessages, fmt.Sprintf("Row %d: %s", rowNum, err.Error()))
				continue
			}
		}

		// Create new record using model type
		recordMap["id"] = uuid.New()

		// Create instance of the model type and populate it via reflection
		modelType := reflect.TypeOf(config.Model).Elem()
		newRecordVal := reflect.New(modelType).Elem()

		// Populate struct fields from recordMap
		for key, val := range recordMap {
			// Convert snake_case to PascalCase for struct field names
			fieldName := snakeToPascal(key)
			field := newRecordVal.FieldByName(fieldName)
			if !field.IsValid() || !field.CanSet() {
				continue
			}

			// Set the value based on type
			if val != nil {
				valReflect := reflect.ValueOf(val)
				if valReflect.Type().AssignableTo(field.Type()) {
					field.Set(valReflect)
				} else if valReflect.Type().ConvertibleTo(field.Type()) {
					field.Set(valReflect.Convert(field.Type()))
				}
			}
		}

		// Use GORM to create the populated struct - this handles PostgreSQL properly
		newRecord := newRecordVal.Addr().Interface()
		if err := a.DB.Create(newRecord).Error; err != nil {
			errors++
			errorMessages = append(errorMessages, fmt.Sprintf("Row %d: failed to create - %s", rowNum, err.Error()))
			continue
		}
		created++
	}

	return r.SendEnvelope(map[string]any{
		"created":  created,
		"updated":  updated,
		"skipped":  skipped,
		"errors":   errors,
		"messages": errorMessages,
	})
}

// importContactSnapshot replaces an organization's active contact list with the
// uploaded file in one transaction. Existing rows are matched only by ID; an
// empty ID creates a new contact. Any validation or database error rolls the
// whole operation back, so a partial file can never delete valid contacts.
func (a *App) importContactSnapshot(
	r *fastglue.Request,
	reader *csv.Reader,
	config ImportConfig,
	columnIndex map[string]int,
	orgID uuid.UUID,
) error {
	if _, ok := columnIndex["id"]; !ok {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "ID column is required for full contact list replacement", nil, "")
	}

	tx := a.DB.Begin()
	if tx.Error != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to start contact import", nil, "")
	}
	defer tx.Rollback() //nolint:errcheck

	const maxImportRows = 10000
	var created, updated, errors, dataRows int
	var errorMessages []string
	keptIDs := make([]uuid.UUID, 0)
	seenIDs := make(map[uuid.UUID]bool)

	rowNum := 1
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		rowNum++
		if rowNum > maxImportRows+1 {
			errors++
			errorMessages = append(errorMessages, fmt.Sprintf("Import limited to %d rows", maxImportRows))
			break
		}
		if err != nil {
			errors++
			errorMessages = append(errorMessages, fmt.Sprintf("Row %d: failed to parse", rowNum))
			continue
		}
		if spreadsheetRowIsEmpty(record) {
			continue
		}
		dataRows++

		recordMap := map[string]any{"organization_id": orgID}
		rowHasError := false
		for col, idx := range columnIndex {
			if idx >= len(record) {
				continue
			}
			value := repairExcelMojibake(strings.TrimSpace(record[idx]))
			if len(value) > 10000 {
				errors++
				rowHasError = true
				errorMessages = append(errorMessages, fmt.Sprintf("Row %d: %s exceeds max length", rowNum, col))
				break
			}

			if transform, ok := config.ColumnTransform[col]; ok {
				transformed, transformErr := transform(value)
				if transformErr != nil {
					errors++
					rowHasError = true
					errorMessages = append(errorMessages, fmt.Sprintf("Row %d: %s - %s", rowNum, col, transformErr.Error()))
					break
				}
				if transformed != nil {
					recordMap[col] = transformed
				} else if col != "id" {
					recordMap[col] = contactSnapshotEmptyValue(col)
				}
			} else {
				// In snapshot mode an empty cell intentionally clears that field.
				recordMap[col] = value
			}
		}
		if rowHasError {
			continue
		}

		country, _ := recordMap["phone_country_code"].(string)
		phone, _ := recordMap["phone_number"].(string)
		normalizedPhone, normalizeErr := normalizeImportedPhone(country, phone)
		if normalizeErr != nil {
			errors++
			errorMessages = append(errorMessages, fmt.Sprintf("Row %d: phone_number - %s", rowNum, normalizeErr.Error()))
			continue
		}
		recordMap["phone_number"] = normalizedPhone

		recordID, hasID := recordMap["id"].(uuid.UUID)
		if hasID {
			if seenIDs[recordID] {
				errors++
				errorMessages = append(errorMessages, fmt.Sprintf("Row %d: duplicate ID %s", rowNum, recordID))
				continue
			}
			seenIDs[recordID] = true

			var existing models.Contact
			if err := tx.Unscoped().Where("organization_id = ? AND id = ?", orgID, recordID).First(&existing).Error; err != nil {
				errors++
				errorMessages = append(errorMessages, fmt.Sprintf("Row %d: ID %s does not belong to an existing contact", rowNum, recordID))
				continue
			}

			if conflict := contactPhoneConflict(tx, orgID, normalizedPhone, recordID); conflict != nil {
				errors++
				errorMessages = append(errorMessages, fmt.Sprintf("Row %d: phone number belongs to contact ID %s", rowNum, conflict.ID))
				continue
			}

			setContactSnapshotDisplayName(recordMap, columnIndex)
			delete(recordMap, "id")
			delete(recordMap, "organization_id")
			recordMap["deleted_at"] = nil
			if err := tx.Unscoped().Model(&existing).Updates(recordMap).Error; err != nil {
				errors++
				errorMessages = append(errorMessages, fmt.Sprintf("Row %d: failed to update contact", rowNum))
				break
			}
			updated++
			keptIDs = append(keptIDs, recordID)
			continue
		}

		if conflict := contactPhoneConflict(tx, orgID, normalizedPhone, uuid.Nil); conflict != nil {
			errors++
			errorMessages = append(errorMessages,
				fmt.Sprintf("Row %d: ID is empty but phone number already belongs to contact ID %s; download the current list again", rowNum, conflict.ID))
			continue
		}

		newID := uuid.New()
		recordMap["id"] = newID
		setContactSnapshotDisplayName(recordMap, columnIndex)
		newContact := &models.Contact{}
		populateImportModel(reflect.ValueOf(newContact).Elem(), recordMap)
		if err := tx.Create(newContact).Error; err != nil {
			errors++
			errorMessages = append(errorMessages, fmt.Sprintf("Row %d: failed to create contact", rowNum))
			break
		}
		created++
		keptIDs = append(keptIDs, newID)
	}

	if dataRows == 0 {
		tx.Rollback() //nolint:errcheck
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "The contact file must contain at least one data row", nil, "")
	}
	if errors > 0 {
		tx.Rollback() //nolint:errcheck
		errorMessages = append(errorMessages, "No changes were applied because the file contains errors.")
		return r.SendEnvelope(map[string]any{
			"created":  0,
			"updated":  0,
			"deleted":  0,
			"skipped":  0,
			"errors":   errors,
			"messages": errorMessages,
		})
	}

	deleteQuery := tx.Where("organization_id = ?", orgID)
	if len(keptIDs) > 0 {
		deleteQuery = deleteQuery.Where("id NOT IN ?", keptIDs)
	}
	deleteResult := deleteQuery.Delete(&models.Contact{})
	if deleteResult.Error != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to remove contacts missing from the file", nil, "")
	}
	if err := tx.Commit().Error; err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, "Failed to commit contact list replacement", nil, "")
	}

	return r.SendEnvelope(map[string]any{
		"created":  created,
		"updated":  updated,
		"deleted":  deleteResult.RowsAffected,
		"skipped":  0,
		"errors":   0,
		"messages": []string{},
	})
}

func spreadsheetRowIsEmpty(record []string) bool {
	for _, value := range record {
		if strings.TrimSpace(value) != "" {
			return false
		}
	}
	return true
}

func contactSnapshotEmptyValue(column string) any {
	switch column {
	case "purchase_score":
		return 0
	case "has_purchased":
		return false
	case "tags":
		return models.JSONBArray{}
	case "assigned_user_id":
		return nil
	default:
		return ""
	}
}

func setContactSnapshotDisplayName(record map[string]any, columnIndex map[string]int) {
	_, hasFirstName := columnIndex["first_name"]
	_, hasLastName := columnIndex["last_name"]
	if !hasFirstName && !hasLastName {
		return
	}
	firstName, _ := record["first_name"].(string)
	lastName, _ := record["last_name"].(string)
	if fullName := strings.TrimSpace(firstName + " " + lastName); fullName != "" {
		record["profile_name"] = fullName
		return
	}
	if _, hasProfileName := columnIndex["profile_name"]; !hasProfileName {
		record["profile_name"] = ""
	}
}

func contactPhoneConflict(db *gorm.DB, orgID uuid.UUID, phone string, excludedID uuid.UUID) *models.Contact {
	query := db.Unscoped().
		Where("organization_id = ? AND phone_number IN ?", orgID, []string{phone, "+" + phone})
	if excludedID != uuid.Nil {
		query = query.Where("id <> ?", excludedID)
	}
	var conflict models.Contact
	if err := query.First(&conflict).Error; err == nil {
		return &conflict
	}
	return nil
}

func populateImportModel(modelValue reflect.Value, record map[string]any) {
	for key, value := range record {
		field := modelValue.FieldByName(snakeToPascal(key))
		if !field.IsValid() || !field.CanSet() || value == nil {
			continue
		}
		valueReflect := reflect.ValueOf(value)
		if valueReflect.Type().AssignableTo(field.Type()) {
			field.Set(valueReflect)
		} else if valueReflect.Type().ConvertibleTo(field.Type()) {
			field.Set(valueReflect.Convert(field.Type()))
		}
	}
}

// GetExportConfig returns the export configuration for a table
func (a *App) GetExportConfig(r *fastglue.Request) error {
	orgID, userID, err := a.getOrgAndUserID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}

	tableName := r.RequestCtx.UserValue("table").(string)

	config, ok := exportConfigs[tableName]
	if !ok {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Invalid table", nil, "")
	}

	// Check permission
	if !a.HasPermission(userID, config.Resource, models.ActionExport, orgID) {
		return r.SendErrorEnvelope(fasthttp.StatusForbidden, "You do not have permission to export "+tableName, nil, "")
	}

	// Build column info
	columns := make([]map[string]string, len(config.AllowedColumns))
	for i, col := range config.AllowedColumns {
		label := col
		if l, ok := config.ColumnLabels[col]; ok {
			label = l
		}
		columns[i] = map[string]string{
			"key":   col,
			"label": label,
		}
	}

	return r.SendEnvelope(map[string]any{
		"table":           tableName,
		"columns":         columns,
		"default_columns": config.DefaultColumns,
	})
}

// GetImportConfig returns the import configuration for a table
func (a *App) GetImportConfig(r *fastglue.Request) error {
	orgID, userID, err := a.getOrgAndUserID(r)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
	}

	tableName := r.RequestCtx.UserValue("table").(string)

	config, ok := importConfigs[tableName]
	if !ok {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Invalid table", nil, "")
	}

	// Check permission
	if !a.HasPermission(userID, config.Resource, models.ActionImport, orgID) {
		return r.SendErrorEnvelope(fasthttp.StatusForbidden, "You do not have permission to import "+tableName, nil, "")
	}

	// Get labels from export config if available
	var columnLabels map[string]string
	if expConfig, ok := exportConfigs[tableName]; ok {
		columnLabels = expConfig.ColumnLabels
	}

	// Build column info
	requiredCols := make([]map[string]string, len(config.RequiredColumns))
	for i, col := range config.RequiredColumns {
		label := col
		if columnLabels != nil {
			if l, ok := columnLabels[col]; ok {
				label = l
			}
		}
		requiredCols[i] = map[string]string{
			"key":   col,
			"label": label,
		}
	}

	optionalCols := make([]map[string]string, len(config.OptionalColumns))
	for i, col := range config.OptionalColumns {
		label := col
		if columnLabels != nil {
			if l, ok := columnLabels[col]; ok {
				label = l
			}
		}
		optionalCols[i] = map[string]string{
			"key":   col,
			"label": label,
		}
	}

	return r.SendEnvelope(map[string]any{
		"table":            tableName,
		"required_columns": requiredCols,
		"optional_columns": optionalCols,
		"unique_column":    config.UniqueColumn,
	})
}

// Helper function to convert snake_case to PascalCase
// Handles common acronyms like ID, URL, API, etc.
func snakeToPascal(s string) string {
	// Common acronyms that should be all uppercase
	acronyms := map[string]string{
		"id":   "ID",
		"url":  "URL",
		"api":  "API",
		"uuid": "UUID",
		"ip":   "IP",
		"http": "HTTP",
		"sql":  "SQL",
		"json": "JSON",
	}

	parts := strings.Split(s, "_")
	for i, part := range parts {
		lower := strings.ToLower(part)
		if acronym, ok := acronyms[lower]; ok {
			parts[i] = acronym
		} else if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, "")
}

// Helper function to format values for CSV export
func formatExportValue(v any) string {
	if v == nil {
		return ""
	}

	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return string(val)
	case int, int32, int64:
		return fmt.Sprintf("%d", val)
	case float32, float64:
		return fmt.Sprintf("%f", val)
	case bool:
		if val {
			return "true"
		}
		return "false"
	case time.Time:
		return val.Format(time.RFC3339)
	case *time.Time:
		if val != nil {
			return val.Format(time.RFC3339)
		}
		return ""
	case uuid.UUID:
		return val.String()
	case *uuid.UUID:
		if val != nil {
			return val.String()
		}
		return ""
	default:
		// Try JSON marshal for complex types
		if b, err := json.Marshal(val); err == nil {
			return string(b)
		}
		return fmt.Sprintf("%v", val)
	}
}

// Helper method to get column label
func (c ImportConfig) getColumnLabel(col string) string {
	if expConfig, ok := exportConfigs[c.Resource]; ok {
		if label, ok := expConfig.ColumnLabels[col]; ok {
			return label
		}
	}
	return col
}

func (c ImportConfig) matchesAlias(column, header string) bool {
	for _, alias := range c.ColumnAliases[column] {
		if strings.EqualFold(strings.TrimSpace(alias), strings.TrimSpace(header)) {
			return true
		}
	}
	return false
}

func digitsOnly(value string) string {
	var b strings.Builder
	for _, r := range value {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// normalizeImportedPhone produces the same digit-only WhatsApp identifier for
// the common spreadsheet variants:
//   - country code 90 + phone 5551112233
//   - country code 90 + phone 05551112233
//   - phone +905551112233 / 905551112233
func normalizeImportedPhone(countryCode, phoneNumber string) (string, error) {
	country := digitsOnly(countryCode)
	phone := digitsOnly(phoneNumber)
	if phone == "" {
		return "", fmt.Errorf("phone number is required")
	}

	if country == "" || strings.HasPrefix(phone, country) {
		return phone, nil
	}
	if international := strings.TrimPrefix(phone, "00"); international != phone && strings.HasPrefix(international, country) {
		return international, nil
	}

	// A leading zero is a domestic trunk prefix, not part of the international
	// WhatsApp identifier when a country code is supplied separately.
	phone = strings.TrimLeft(phone, "0")
	if phone == "" {
		return "", fmt.Errorf("phone number is required")
	}
	return country + phone, nil
}

// applyImportedContactDisplayName keeps the display name synchronized with
// first_name/last_name. On updates, an omitted name component is taken from the
// existing contact, so changing only the surname does not erase the first name.
func applyImportedContactDisplayName(record map[string]any, existing *models.Contact) {
	if _, explicitlySet := record["profile_name"]; explicitlySet {
		return
	}

	firstValue, firstProvided := record["first_name"]
	lastValue, lastProvided := record["last_name"]
	if !firstProvided && !lastProvided {
		return
	}

	firstName := ""
	lastName := ""
	if existing != nil {
		firstName = existing.FirstName
		lastName = existing.LastName
	}
	if firstProvided {
		firstName = strings.TrimSpace(fmt.Sprint(firstValue))
	}
	if lastProvided {
		lastName = strings.TrimSpace(fmt.Sprint(lastValue))
	}

	if fullName := strings.TrimSpace(firstName + " " + lastName); fullName != "" {
		record["profile_name"] = fullName
	}
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

func normalizeSpreadsheetUpload(data []byte, filename string) ([]byte, error) {
	isXLSX := strings.EqualFold(filepath.Ext(filename), ".xlsx") ||
		bytes.HasPrefix(data, []byte{'P', 'K', 0x03, 0x04})
	if !isXLSX {
		return data, nil
	}

	workbook, err := excelize.OpenReader(bytes.NewReader(data), excelize.Options{
		UnzipSizeLimit:    64 << 20,
		UnzipXMLSizeLimit: 16 << 20,
	})
	if err != nil {
		return nil, fmt.Errorf("invalid Excel workbook")
	}
	defer workbook.Close() //nolint:errcheck

	sheets := workbook.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("Excel workbook does not contain a worksheet")
	}
	rows, err := workbook.GetRows(sheets[0])
	if err != nil {
		return nil, fmt.Errorf("failed to read Excel worksheet")
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("Excel worksheet is empty")
	}

	var csvBuffer bytes.Buffer
	csvBuffer.WriteString("sep=;\r\n")
	writer := csv.NewWriter(&csvBuffer)
	writer.Comma = ';'
	writer.UseCRLF = true
	for _, row := range rows {
		if err := writer.Write(row); err != nil {
			return nil, fmt.Errorf("failed to convert Excel worksheet")
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("failed to convert Excel worksheet")
	}
	return csvBuffer.Bytes(), nil
}

// decodeSpreadsheetCSV converts UTF-16 CSV files produced for Windows Excel
// into UTF-8 before they are passed to encoding/csv. UTF-8 and ASCII files are
// returned unchanged.
func decodeSpreadsheetCSV(data []byte) ([]byte, error) {
	littleEndian := bytes.HasPrefix(data, []byte{0xff, 0xfe})
	bigEndian := bytes.HasPrefix(data, []byte{0xfe, 0xff})
	if !littleEndian && !bigEndian {
		if utf8.Valid(data) {
			return data, nil
		}

		// Turkish Windows Excel commonly saves "CSV (Comma delimited)" as
		// Windows-1254. Convert that legacy encoding automatically instead of
		// requiring users to repair names and company fields one by one.
		decoded, _, err := transform.Bytes(charmap.Windows1254.NewDecoder(), data)
		if err != nil || !utf8.Valid(decoded) {
			return nil, fmt.Errorf("unsupported spreadsheet character encoding")
		}
		return decoded, nil
	}

	data = data[2:]
	if len(data)%2 != 0 {
		return nil, fmt.Errorf("invalid UTF-16 byte length")
	}

	codeUnits := make([]uint16, len(data)/2)
	for i := range codeUnits {
		if littleEndian {
			codeUnits[i] = uint16(data[i*2]) | uint16(data[i*2+1])<<8
		} else {
			codeUnits[i] = uint16(data[i*2])<<8 | uint16(data[i*2+1])
		}
	}
	decoded := []byte(string(utf16.Decode(codeUnits)))
	if !utf8.Valid(decoded) {
		return nil, fmt.Errorf("invalid UTF-16 text")
	}
	return decoded, nil
}

// repairExcelMojibake fixes reversible legacy text corruption created when
// UTF-8 Turkish text was previously interpreted as a Windows code page. The
// candidate is accepted only when it contains fewer mojibake markers, so
// normal Turkish and other valid Unicode text is left unchanged.
func repairExcelMojibake(value string) string {
	best := value
	bestScore := mojibakeScore(value)
	if bestScore == 0 {
		return value
	}

	encoders := []*charmap.Charmap{charmap.Windows1252, charmap.Windows1254}
	for round := 0; round < 2; round++ {
		changed := false
		for _, codePage := range encoders {
			candidateBytes, _, err := transform.Bytes(codePage.NewEncoder(), []byte(best))
			if err != nil || !utf8.Valid(candidateBytes) {
				continue
			}
			candidate := string(candidateBytes)
			score := mojibakeScore(candidate)
			if score < bestScore {
				best = candidate
				bestScore = score
				changed = true
			}
		}
		if !changed || bestScore == 0 {
			break
		}
	}
	return best
}

func mojibakeScore(value string) int {
	score := 0
	for _, marker := range []string{"Ã", "Ä", "Å", "Â", "Ð", "Ý", "Þ", "�"} {
		score += strings.Count(value, marker)
	}
	for _, r := range value {
		if r >= 0x80 && r <= 0x9f {
			score += 2
		}
	}
	return score
}
