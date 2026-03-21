package export

import (
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

type CellType string

const (
	CellText     CellType = "text"
	CellNumber   CellType = "number"
	CellCurrency CellType = "currency"
	CellDate     CellType = "date"
	CellPercent  CellType = "percent"
)

type CellValue struct {
	Value interface{}
	Type  CellType
}

type ExcelReport struct {
	file        *excelize.File
	module      string
	title       string
	theme       moduleTheme
	widths      map[string]map[int]float64
	headerStyle int
	textStyle   int
	numberStyle int
	moneyStyle  int
	dateStyle   int
	percentStyle int
}

func NewExcelReport(title, module string) *ExcelReport {
	theme := themeForModule(module)
	report := &ExcelReport{
		file:   excelize.NewFile(),
		module: strings.TrimSpace(module),
		title:  strings.TrimSpace(title),
		theme:  theme,
		widths: map[string]map[int]float64{},
	}

	report.file.SetDocProps(&excelize.DocProperties{
		Creator: "KANTOR",
		Title:   report.title,
		Subject: theme.Label,
	})

	report.headerStyle, _ = report.file.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Color: "#FFFFFF"},
		Fill: excelize.Fill{
			Type:    "pattern",
			Color:   []string{theme.HeaderHex},
			Pattern: 1,
		},
		Alignment: &excelize.Alignment{
			Horizontal: "center",
			Vertical:   "center",
		},
	})
	report.textStyle, _ = report.file.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{Vertical: "center"},
	})
	report.numberStyle, _ = report.file.NewStyle(&excelize.Style{
		NumFmt: 3,
	})
	report.moneyStyle, _ = report.file.NewStyle(&excelize.Style{
		CustomNumFmt: &[]string{"[$Rp-421] #,##0"}[0],
	})
	report.dateStyle, _ = report.file.NewStyle(&excelize.Style{
		CustomNumFmt: &[]string{"dd/mm/yyyy"}[0],
	})
	report.percentStyle, _ = report.file.NewStyle(&excelize.Style{
		CustomNumFmt: &[]string{"0.00%"}[0],
	})

	defaultSheet := report.file.GetSheetName(0)
	report.file.SetSheetName(defaultSheet, "Report")
	return report
}

func (r *ExcelReport) AddSheet(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		trimmed = fmt.Sprintf("Sheet%d", len(r.file.GetSheetList())+1)
	}
	if existing, err := r.file.GetSheetIndex(trimmed); err == nil && existing != -1 {
		return trimmed
	}
	r.file.NewSheet(trimmed)
	return trimmed
}

func (r *ExcelReport) WriteHeader(sheet string, row int, headers []string) error {
	for index, header := range headers {
		cell, err := excelize.CoordinatesToCellName(index+1, row)
		if err != nil {
			return err
		}
		if err := r.file.SetCellValue(sheet, cell, header); err != nil {
			return err
		}
		r.file.SetCellStyle(sheet, cell, cell, r.headerStyle)
		r.trackWidth(sheet, index+1, header)
	}
	return nil
}

func (r *ExcelReport) WriteRows(sheet string, startRow int, data [][]CellValue) error {
	for rowOffset, row := range data {
		for colIndex, cellValue := range row {
			cellName, err := excelize.CoordinatesToCellName(colIndex+1, startRow+rowOffset)
			if err != nil {
				return err
			}

			value := cellValue.Value
			switch cellValue.Type {
			case CellDate:
				if timeValue, ok := value.(time.Time); ok {
					value = timeValue
				}
				r.file.SetCellStyle(sheet, cellName, cellName, r.dateStyle)
			case CellCurrency:
				r.file.SetCellStyle(sheet, cellName, cellName, r.moneyStyle)
			case CellNumber:
				r.file.SetCellStyle(sheet, cellName, cellName, r.numberStyle)
			case CellPercent:
				r.file.SetCellStyle(sheet, cellName, cellName, r.percentStyle)
			default:
				r.file.SetCellStyle(sheet, cellName, cellName, r.textStyle)
			}

			if err := r.file.SetCellValue(sheet, cellName, value); err != nil {
				return err
			}
			r.trackWidth(sheet, colIndex+1, cellValue.displayValue())
		}
	}

	return nil
}

func (r *ExcelReport) AddSummarySheet(data map[string]string) error {
	if len(data) == 0 {
		return nil
	}

	sheet := r.AddSheet("Summary")
	if err := r.WriteHeader(sheet, 1, []string{"Metric", "Value"}); err != nil {
		return err
	}

	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	rows := make([][]CellValue, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, []CellValue{
			{Value: key, Type: CellText},
			{Value: data[key], Type: CellText},
		})
	}

	return r.WriteRows(sheet, 2, rows)
}

func (r *ExcelReport) Save(writer io.Writer) error {
	for sheet, cols := range r.widths {
		for colIndex, width := range cols {
			colName, err := excelize.ColumnNumberToName(colIndex)
			if err != nil {
				return err
			}
			if err := r.file.SetColWidth(sheet, colName, colName, math.Min(width+2, 60)); err != nil {
				return err
			}
		}
	}

	return r.file.Write(writer)
}

func TextCell(value interface{}) CellValue {
	return CellValue{Value: value, Type: CellText}
}

func NumberCell(value interface{}) CellValue {
	return CellValue{Value: value, Type: CellNumber}
}

func CurrencyCell(value interface{}) CellValue {
	return CellValue{Value: value, Type: CellCurrency}
}

func DateCell(value time.Time) CellValue {
	return CellValue{Value: value, Type: CellDate}
}

func PercentCell(value float64) CellValue {
	return CellValue{Value: value, Type: CellPercent}
}

func (r *ExcelReport) trackWidth(sheet string, column int, value string) {
	if _, ok := r.widths[sheet]; !ok {
		r.widths[sheet] = map[int]float64{}
	}

	width := float64(len([]rune(strings.TrimSpace(value))))
	if width < 10 {
		width = 10
	}
	if width > r.widths[sheet][column] {
		r.widths[sheet][column] = width
	}
}

func (c CellValue) displayValue() string {
	switch value := c.Value.(type) {
	case nil:
		return ""
	case string:
		return value
	case time.Time:
		return value.Format("02/01/2006")
	default:
		return fmt.Sprintf("%v", value)
	}
}

type moduleTheme struct {
	Label     string
	HeaderHex string
	HeaderRGB [3]int
}

func themeForModule(module string) moduleTheme {
	switch strings.ToLower(strings.TrimSpace(module)) {
	case "operational":
		return moduleTheme{Label: "Operasional", HeaderHex: "#0065FF", HeaderRGB: [3]int{0, 101, 255}}
	case "hris":
		return moduleTheme{Label: "HRIS", HeaderHex: "#6554C0", HeaderRGB: [3]int{101, 84, 192}}
	case "marketing":
		return moduleTheme{Label: "Marketing", HeaderHex: "#FF5630", HeaderRGB: [3]int{255, 86, 48}}
	case "admin":
		return moduleTheme{Label: "Admin", HeaderHex: "#DE350B", HeaderRGB: [3]int{222, 53, 11}}
	default:
		return moduleTheme{Label: "Report", HeaderHex: "#42526E", HeaderRGB: [3]int{66, 82, 110}}
	}
}
